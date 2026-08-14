package heartbeat

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nixys/nxs-backup/misc"
	"github.com/nixys/nxs-backup/modules/logger"
)

// Heartbeats collects results of the performed jobs
// and pings the configured monitoring services
type Heartbeats struct {
	mu        sync.Mutex
	list      []*Heartbeat
	startedAt time.Time
	jobs      []jobResult
}

type jobResult struct {
	name    string
	skipped bool
	err     error
}

func InitAll(list []*Heartbeat) *Heartbeats {
	if len(list) == 0 {
		return nil
	}
	return &Heartbeats{
		list:      list,
		startedAt: time.Now(),
	}
}

// Start pings the heartbeats that report about the beginning of the backup run
func (hs *Heartbeats) Start(logCh chan logger.LogRecord) {
	if hs == nil {
		return
	}

	hs.mu.Lock()
	defer hs.mu.Unlock()

	hs.startedAt = time.Now()
	for _, h := range hs.list {
		h.Start(logCh)
	}
}

// AddJob registers the result of the performed job
func (hs *Heartbeats) AddJob(name string, skipped bool, err error) {
	if hs == nil {
		return
	}

	hs.mu.Lock()
	defer hs.mu.Unlock()

	hs.jobs = append(hs.jobs, jobResult{name: name, skipped: skipped, err: err})
}

// Perform pings all the configured heartbeats with the result of the backup run
func (hs *Heartbeats) Perform(logCh chan logger.LogRecord, generalErr error) {
	if hs == nil {
		return
	}

	hs.mu.Lock()
	defer hs.mu.Unlock()

	d := time.Since(hs.startedAt)

	for _, h := range hs.list {
		st, ok := hs.status(h.JobsFilter(), generalErr)
		if !ok {
			logCh <- logger.Log("", "").Debugf("Heartbeat `%s` is not pinged: none of its jobs has been performed.", h.Name())
			continue
		}
		st.Duration = d
		h.Ping(logCh, st)
	}
}

// status calculates the run result the heartbeat has to report about.
// For the heartbeat bound to the certain jobs only their results are taken into
// account, and the second return value is false if none of them has been performed.
func (hs *Heartbeats) status(jobs []string, generalErr error) (Status, bool) {
	var (
		performed int
		failed    []string
		firstErr  error
	)

	for _, j := range hs.jobs {
		if len(jobs) > 0 && !misc.Contains(jobs, j.name) {
			continue
		}
		if j.skipped {
			continue
		}

		performed++
		if j.err != nil {
			failed = append(failed, j.name)
			if firstErr == nil {
				firstErr = j.err
			}
		}
	}

	if len(jobs) > 0 {
		if performed == 0 {
			return Status{}, false
		}
	} else if generalErr != nil {
		// the run itself failed, e.g. another nxs-backup process is already running
		return Status{
			Ok:      false,
			Message: fmt.Sprintf("nxs-backup: %s", firstLine(generalErr)),
		}, true
	}

	if len(failed) > 0 {
		return Status{
			Ok: false,
			Message: fmt.Sprintf("nxs-backup: %d of %d jobs failed (%s): %s",
				len(failed), performed, strings.Join(failed, ", "), firstLine(firstErr)),
		}, true
	}

	return Status{
		Ok:      true,
		Message: fmt.Sprintf("nxs-backup: %d jobs finished successfully", performed),
	}, true
}

// wrappedErrors is implemented by the multierror.Error the jobs return
type wrappedErrors interface {
	WrappedErrors() []error
}

// firstLine returns a short description of the error to be reported
// to the monitoring service
func firstLine(err error) string {
	if err == nil {
		return ""
	}

	// the multierror text starts with the `N errors occurred:` header,
	// so the wrapped error is more informative
	for {
		we, ok := err.(wrappedErrors)
		if !ok || len(we.WrappedErrors()) == 0 {
			break
		}
		err = we.WrappedErrors()[0]
	}

	for _, l := range strings.Split(err.Error(), "\n") {
		l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "* "))
		if l != "" {
			return l
		}
	}

	return err.Error()
}
