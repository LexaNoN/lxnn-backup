package report

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/nixys/nxs-backup/misc"
	"github.com/nixys/nxs-backup/modules/logger"
	"github.com/nixys/nxs-backup/modules/metrics"
)

// Sender delivers the final report to the configured channel
type Sender interface {
	Name() string
	Send(text string) error
}

// Opts contains final report options
type Opts struct {
	ProjectName    string
	ServerName     string
	StateFile      string
	OnlyOnErrors   bool
	IncludeTargets bool
	Sender         Sender
	// AlertSender receives the report only if the run has problems. Optional.
	AlertSender Sender
}

// Report collects results of the performed jobs and sends the summary
type Report struct {
	opts      Opts
	mu        sync.Mutex
	startedAt time.Time
	jobs      []JobResult
	prevState *state
	stateErr  error
}

// JobResult contains the result of a single performed job
type JobResult struct {
	Name     string
	Type     misc.BackupType
	Skipped  bool
	Duration time.Duration
	Err      error
}

func Init(o Opts) (*Report, error) {
	if o.Sender == nil {
		return nil, fmt.Errorf("Report init fail. Notification channel is not configured. ")
	}

	r := &Report{
		opts:      o,
		startedAt: time.Now(),
	}
	// the state of the previous run has to be read before the backups are started
	r.prevState, r.stateErr = loadState(o.StateFile)

	return r, nil
}

// Start marks the beginning of the backup process
func (r *Report) Start() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.startedAt = time.Now()
}

// AddJob registers the result of the performed job
func (r *Report) AddJob(res JobResult) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs = append(r.jobs, res)
}

// Perform builds the final report, sends it to the configured channel
// and stores the current backups sizes for the next run
func (r *Report) Perform(logCh chan logger.LogRecord, md *metrics.Data, generalErr error) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	d := r.collect(md, generalErr)

	msgs := buildMessages(d)

	if r.opts.AlertSender != nil && d.hasProblems() {
		send(r.opts.AlertSender, msgs, logCh)
	}

	if r.opts.OnlyOnErrors && !d.hasProblems() {
		logCh <- logger.Log("", "").Debug("Final report is not sent: no errors occurred.")
	} else {
		send(r.opts.Sender, msgs, logCh)
	}

	if err := r.prevState.save(r.opts.StateFile, md); err != nil {
		logCh <- logger.Log("", "").Errorf("Failed to save final report state file: %v", err)
	}
}

func send(s Sender, msgs []string, logCh chan logger.LogRecord) {
	for _, m := range msgs {
		if err := s.Send(m); err != nil {
			logCh <- logger.Log("", "").Errorf("Failed to send final report via %s: %v", s.Name(), err)
			return
		}
	}
	logCh <- logger.Log("", "").Debugf("Final report sent via %s.", s.Name())
}

func (r *Report) collect(md *metrics.Data, generalErr error) reportData {
	d := reportData{
		Project:        r.opts.ProjectName,
		Server:         r.opts.ServerName,
		Version:        misc.VERSION,
		StartedAt:      r.startedAt,
		Duration:       time.Since(r.startedAt),
		PrevRunAt:      r.prevState.updatedAt(),
		GeneralErr:     generalErr,
		StateErr:       r.stateErr,
		IncludeTargets: r.opts.IncludeTargets,
	}

	for _, res := range r.jobs {
		j := jobInfo{
			Name:     res.Name,
			Type:     string(res.Type),
			Skipped:  res.Skipped,
			Err:      res.Err,
			Duration: res.Duration,
		}

		if md != nil {
			j.Targets = collectTargets(md.Job[res.Name], res.Name, r.prevState)
		}

		for _, t := range j.Targets {
			j.Size += t.Size
			if t.HasDelta {
				j.Delta += t.Delta
				j.HasDelta = true
			}
			if t.Ok {
				j.TargetsOk++
			} else {
				j.TargetsFailed++
			}
		}

		switch {
		case j.Skipped:
			d.JobsSkipped++
		case j.Err != nil || j.TargetsFailed > 0:
			d.JobsFailed++
		default:
			d.JobsOk++
		}

		d.TargetsOk += j.TargetsOk
		d.TargetsFailed += j.TargetsFailed
		d.TotalSize += j.Size
		if j.HasDelta {
			d.TotalDelta += j.Delta
			d.HasDelta = true
		}

		d.Jobs = append(d.Jobs, j)
	}

	return d
}

// collectTargets converts collected metrics of the job targets to the report data
func collectTargets(jd metrics.JobData, jobName string, prev *state) []targetInfo {
	var targets []targetInfo

	names := make([]string, 0, len(jd.TargetMetrics))
	for name := range jd.TargetMetrics {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		tm := jd.TargetMetrics[name]
		// no values means the target wasn't processed during this run
		if len(tm.Values) == 0 {
			continue
		}

		t := targetInfo{
			Name:      name,
			Size:      int64(tm.Values[metrics.BackupSize]),
			Ok:        tm.Values[metrics.BackupOk] == 1,
			Delivered: tm.Values[metrics.DeliveryOk] == 1,
			Duration: time.Duration(tm.Values[metrics.BackupTime])*time.Millisecond +
				time.Duration(tm.Values[metrics.DeliveryTime])*time.Millisecond,
		}

		if t.Ok {
			if prevSize, ok := prev.targetSize(jobName, name); ok {
				t.Delta = t.Size - prevSize
				t.HasDelta = true
			}
		}

		targets = append(targets, t)
	}

	return targets
}
