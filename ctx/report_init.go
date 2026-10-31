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

	tg, err := telegram.Init(tgOpts(rc.Telegram))
	if err != nil {
		return nil, err
	}

	// the alerts channel is optional, it's enabled by setting its own chat
	var alerts report.Sender
	if rc.AlertsTelegram.ChatID != "" {
		ac := rc.AlertsTelegram
		if ac.BotToken == "" {
			ac.BotToken = rc.Telegram.BotToken
		}
		if ac.APIURL == "" {
			ac.APIURL = rc.Telegram.APIURL
		}
		if alerts, err = telegram.Init(tgOpts(ac)); err != nil {
			return nil, err
		}
	}

	return report.Init(report.Opts{
		ProjectName:    conf.ProjectName,
		ServerName:     conf.ServerName,
		StateFile:      rc.StateFile,
		OnlyOnErrors:   rc.OnlyOnErrors,
		IncludeTargets: rc.IncludeTargets,
		Sender:         tg,
		AlertSender:    alerts,
	})
}

func tgOpts(c reportTelegramConf) telegram.Opts {
	return telegram.Opts{
		BotToken:            c.BotToken,
		ChatID:              c.ChatID,
		ThreadID:            c.ThreadID,
		APIURL:              c.APIURL,
		InsecureTLS:         c.InsecureTLS,
		DisableNotification: c.DisableNotification,
	}
}
