package start_backup

import (
	"fmt"
	"os"
	"path"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/nightlyone/lockfile"

	"github.com/nixys/nxs-backup/interfaces"
	"github.com/nixys/nxs-backup/modules/backup"
	"github.com/nixys/nxs-backup/modules/logger"
	"github.com/nixys/nxs-backup/modules/metrics"
	"github.com/nixys/nxs-backup/modules/notifier/heartbeat"
	"github.com/nixys/nxs-backup/modules/report"
)

type Opts struct {
	InitErr     error
	Done        chan error
	EvCh        chan logger.LogRecord
	WaitPrev    time.Duration
	JobName     string
	Jobs        map[string]interfaces.Job
	FileJobs    interfaces.Jobs
	DBJobs      interfaces.Jobs
	ExtJobs     interfaces.Jobs
	MetricsData *metrics.Data
	Report      *report.Report
	Heartbeats  *heartbeat.Heartbeats
}

type startBackup struct {
	initErr     error
	done        chan error
	evCh        chan logger.LogRecord
	waitPrev    time.Duration
	jobName     string
	jobs        map[string]interfaces.Job
	fileJobs    interfaces.Jobs
	dbJobs      interfaces.Jobs
	extJobs     interfaces.Jobs
	metricsData *metrics.Data
	report      *report.Report
	heartbeats  *heartbeat.Heartbeats
}

func Init(o Opts) *startBackup {
	return &startBackup{
		initErr:     o.InitErr,
		done:        o.Done,
		evCh:        o.EvCh,
		waitPrev:    o.WaitPrev,
		jobName:     o.JobName,
		jobs:        o.Jobs,
		fileJobs:    o.FileJobs,
		dbJobs:      o.DBJobs,
		extJobs:     o.ExtJobs,
		metricsData: o.MetricsData,
		report:      o.Report,
		heartbeats:  o.Heartbeats,
	}
}

func (sb *startBackup) Run() {
	var (
		err        error
		errs       *multierror.Error
		generalErr *multierror.Error
	)

	if sb.initErr != nil {
		generalErr = multierror.Append(generalErr, sb.initErr)
	}

	defer func() {
		if err = sb.metricsData.SaveFile(); err != nil {
			sb.evCh <- logger.Log("", "").Errorf("Failed to save metrics to file: %v", err)
			errs = multierror.Append(errs, err)
		}
		sb.report.Perform(sb.evCh, sb.metricsData, generalErr.ErrorOrNil())
		sb.heartbeats.Perform(sb.evCh, generalErr.ErrorOrNil())
		if errs.ErrorOrNil() != nil {
			err = fmt.Errorf("Some of backups failed with next errors:\n%w", errs)
		}
		sb.done <- err
	}()

	sb.report.Start()

	if sb.initErr != nil {
		sb.evCh <- logger.Log("", "").Errorf("Backup plan initialised with errors: %v", sb.initErr)
	}

	sb.evCh <- logger.Log("", "").Info("Backup starting.")

	// Crate lockfile
	lock, err := lockfile.New(path.Join(os.TempDir(), "nxs-backup.lck"))
	if sb.waitPrev != 0 {
		now := time.Now()
		waitTill := now.Add(time.Minute * sb.waitPrev)
		for waitTill.After(time.Now()) {
			if err = lock.TryLock(); err != nil {
				time.Sleep(time.Second * 5)
			} else {
				break
			}
		}
	} else {
		err = lock.TryLock()
	}
	if err != nil {
		err = fmt.Errorf("Can't start nxs-backup. Another nxs-backup process already running. ")
		generalErr = multierror.Append(generalErr, err)
		sb.evCh <- logger.Log("", "").Error(err)
		return
	}
	defer func() { _ = lock.Unlock() }()

	sb.heartbeats.Start(sb.evCh)

	if sb.jobName == "external" || sb.jobName == "all" {
		if len(sb.extJobs) > 0 {
			sb.evCh <- logger.Log("", "").Info("Starting backup external jobs.")
			for _, job := range sb.extJobs {
				if err := sb.performJob(job); err != nil {
					errs = multierror.Append(errs, err)
				}
			}
		} else {
			sb.evCh <- logger.Log("", "").Info("No external jobs.")
		}
	}
	if sb.jobName == "databases" || sb.jobName == "all" {
		if len(sb.dbJobs) > 0 {
			sb.evCh <- logger.Log("", "").Info("Starting backup databases jobs.")
			for _, job := range sb.dbJobs {
				if err := sb.performJob(job); err != nil {
					errs = multierror.Append(errs, err)
				}
			}
		} else {
			sb.evCh <- logger.Log("", "").Info("No databases jobs.")
		}
	}
	if sb.jobName == "files" || sb.jobName == "all" {
		if len(sb.fileJobs) > 0 {
			sb.evCh <- logger.Log("", "").Info("Starting backup files jobs.")
			for _, job := range sb.fileJobs {
				if err := sb.performJob(job); err != nil {
					errs = multierror.Append(errs, err)
				}
			}
		} else {
			sb.evCh <- logger.Log("", "").Info("No files jobs.")
		}
	}

	if job, ok := sb.jobs[sb.jobName]; ok {
		if err = sb.performJob(job); err != nil {
			errs = multierror.Append(errs, err)
		}
	}

	sb.evCh <- logger.Log("", "").Infof("Backup finished.\n")
}

// performJob performs the job and registers its result
// in the final report and the heartbeats
func (sb *startBackup) performJob(job interfaces.Job) error {
	startTime := time.Now()
	skipped := !job.NeedToMakeBackup()

	err := backup.Perform(sb.evCh, job)

	sb.report.AddJob(report.JobResult{
		Name:     job.GetName(),
		Type:     job.GetType(),
		Skipped:  skipped,
		Duration: time.Since(startTime),
		Err:      err,
	})
	sb.heartbeats.AddJob(job.GetName(), skipped, err)

	return err
}
