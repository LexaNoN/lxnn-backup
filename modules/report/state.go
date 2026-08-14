package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path"
	"time"

	"github.com/nixys/nxs-backup/modules/metrics"
)

// state keeps sizes of the backups created by the previous run.
// It's used to calculate how much the backups have grown since then.
type state struct {
	UpdatedAt int64               `json:"updated_at"`
	Jobs      map[string]jobState `json:"jobs"`
}

type jobState struct {
	Targets map[string]targetState `json:"targets"`
}

type targetState struct {
	Size      int64 `json:"size"`
	Timestamp int64 `json:"timestamp"`
}

func newState() *state {
	return &state{Jobs: make(map[string]jobState)}
}

// loadState reads the state file. A missing or empty file isn't an error,
// it just means there is nothing to compare the current run with.
func loadState(filePath string) (*state, error) {
	s := newState()

	if filePath == "" {
		return s, nil
	}

	b, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return s, err
	}

	if len(bytes.TrimSpace(b)) == 0 {
		return s, nil
	}

	if err = json.Unmarshal(b, s); err != nil {
		return newState(), err
	}
	if s.Jobs == nil {
		s.Jobs = make(map[string]jobState)
	}

	return s, nil
}

func (s *state) updatedAt() time.Time {
	if s == nil || s.UpdatedAt == 0 {
		return time.Time{}
	}
	return time.Unix(s.UpdatedAt, 0)
}

func (s *state) targetSize(jobName, target string) (int64, bool) {
	if s == nil {
		return 0, false
	}
	j, ok := s.Jobs[jobName]
	if !ok {
		return 0, false
	}
	t, ok := j.Targets[target]
	if !ok {
		return 0, false
	}
	return t.Size, true
}

// saveState stores sizes of the successfully created backups. Values of the targets
// that were not backed up during this run are inherited from the previous state,
// so the next report compares the last successful backups.
func (s *state) save(filePath string, md *metrics.Data) error {
	if filePath == "" {
		return nil
	}

	ns := &state{
		UpdatedAt: time.Now().Unix(),
		Jobs:      make(map[string]jobState),
	}

	for jobName, job := range md.Job {
		ts := make(map[string]targetState)

		for target, tm := range job.TargetMetrics {
			if tm.Values[metrics.BackupOk] == 1 {
				ts[target] = targetState{
					Size:      int64(tm.Values[metrics.BackupSize]),
					Timestamp: int64(tm.Values[metrics.BackupTimestamp]),
				}
				continue
			}
			// backup wasn't created, keep the previous known size
			if old, ok := s.Jobs[jobName].Targets[target]; ok {
				ts[target] = old
			}
		}

		if len(ts) > 0 {
			ns.Jobs[jobName] = jobState{Targets: ts}
		}
	}

	// keep the state of the jobs that were not run
	for jobName, job := range s.Jobs {
		if _, ok := ns.Jobs[jobName]; !ok {
			ns.Jobs[jobName] = job
		}
	}

	b, err := json.Marshal(ns)
	if err != nil {
		return err
	}

	if dir := path.Dir(filePath); dir != "" && dir != "." {
		if err = os.MkdirAll(dir, 0750); err != nil {
			return err
		}
	}

	return os.WriteFile(filePath, b, 0600)
}
