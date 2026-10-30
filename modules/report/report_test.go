package report

import (
	"errors"
	"testing"

	"github.com/nixys/nxs-backup/modules/logger"
)

type fakeSender struct {
	name string
	sent int
}

func (f *fakeSender) Name() string { return f.name }

func (f *fakeSender) Send(string) error {
	f.sent++
	return nil
}

// TestAlertRouting checks that the alerts channel gets the report only on problems
func TestAlertRouting(t *testing.T) {
	for _, tc := range []struct {
		name            string
		jobErr          error
		onlyOnErrors    bool
		wantMain, wantA int
	}{
		{name: "ok run", wantMain: 1},
		{name: "ok run, only_on_errors", onlyOnErrors: true},
		{name: "failed run", jobErr: errors.New("boom"), wantMain: 1, wantA: 1},
		{name: "failed run, only_on_errors", jobErr: errors.New("boom"), onlyOnErrors: true, wantMain: 1, wantA: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			main, alerts := &fakeSender{name: "main"}, &fakeSender{name: "alerts"}

			r, err := Init(Opts{OnlyOnErrors: tc.onlyOnErrors, Sender: main, AlertSender: alerts})
			if err != nil {
				t.Fatal(err)
			}
			r.AddJob(JobResult{Name: "job", Err: tc.jobErr})

			logCh := make(chan logger.LogRecord, 16)
			r.Perform(logCh, nil, nil)

			if main.sent != tc.wantMain || alerts.sent != tc.wantA {
				t.Fatalf("main sent %d (want %d), alerts sent %d (want %d)",
					main.sent, tc.wantMain, alerts.sent, tc.wantA)
			}
		})
	}
}
