package ctx

import (
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"

	"github.com/nixys/nxs-backup/misc"
	"github.com/nixys/nxs-backup/modules/notifier/heartbeat"
)

// heartbeatsInit initializes the external monitorings pinged on the backup run.
// Returns nil if there are no enabled heartbeats.
func heartbeatsInit(conf ConfOpts) (*heartbeat.Heartbeats, error) {
	var (
		errs *multierror.Error
		hbs  []*heartbeat.Heartbeat
	)

	jobNames := make([]string, 0, len(conf.Jobs))
	for _, j := range conf.Jobs {
		jobNames = append(jobNames, j.Name)
	}

	for _, hc := range conf.Notifications.Heartbeats {
		if !hc.Enabled {
			continue
		}

		// a typo in the job name silently turns the heartbeat off, so it's better to fail
		for _, j := range hc.Jobs {
			if !misc.Contains(jobNames, j) {
				errs = multierror.Append(errs, fmt.Errorf("Heartbeat `%s` init fail. Unknown job `%s`. ", hc.Name, j))
			}
		}

		h, err := heartbeat.Init(heartbeat.Opts{
			Name:              hc.Name,
			URL:               hc.URL,
			FailureURL:        hc.FailureURL,
			StartURL:          hc.StartURL,
			Trigger:           strings.ToLower(hc.Trigger),
			Method:            hc.Method,
			Jobs:              hc.Jobs,
			ExtraHeaders:      hc.ExtraHeaders,
			InsecureTLS:       hc.InsecureTLS,
			StatusParams:      hc.StatusParams,
			ConnectionTimeout: hc.ConnectionTimeout,
		})
		if err != nil {
			errs = multierror.Append(errs, err)
			continue
		}

		hbs = append(hbs, h)
	}

	return heartbeat.InitAll(hbs), errs.ErrorOrNil()
}
