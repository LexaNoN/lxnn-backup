package ctx

import (
	"fmt"

	"github.com/nixys/nxs-backup/modules/notifier/telegram"
	"github.com/nixys/nxs-backup/modules/report"
)

// reportInit initializes the final report. Returns nil if the report is disabled by config.
func reportInit(conf ConfOpts) (*report.Report, error) {
	rc := conf.Notifications.Report

	if !rc.Enabled {
		return nil, nil
	}

	if rc.Telegram.BotToken == "" && rc.Telegram.ChatID == "" {
		return nil, fmt.Errorf("Report init fail. Notification channel `telegram` is not configured. ")
	}

	tg, err := telegram.Init(telegram.Opts{
		BotToken:            rc.Telegram.BotToken,
		ChatID:              rc.Telegram.ChatID,
		ThreadID:            rc.Telegram.ThreadID,
		APIURL:              rc.Telegram.APIURL,
		InsecureTLS:         rc.Telegram.InsecureTLS,
		DisableNotification: rc.Telegram.DisableNotification,
	})
	if err != nil {
		return nil, err
	}

	return report.Init(report.Opts{
		ProjectName:    conf.ProjectName,
		ServerName:     conf.ServerName,
		StateFile:      rc.StateFile,
		OnlyOnErrors:   rc.OnlyOnErrors,
		IncludeTargets: rc.IncludeTargets,
		Sender:         tg,
	})
}
