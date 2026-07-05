package main

import (
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/email"
)

// emailTransportConfig maps the app config onto the email transport config used
// to construct the worker-side sender (SMTP, or a temp-file fallback in dev).
func emailTransportConfig(cfg *config.Config) email.TransportConfig {
	return email.TransportConfig{
		SMTPHost:     cfg.SMTPHost,
		SMTPPort:     cfg.SMTPPort,
		SMTPUsername: cfg.SMTPUsername,
		SMTPPassword: cfg.SMTPPassword,
		SMTPTLS:      cfg.SMTPTLS,
		From:         cfg.EmailFrom,
		SiteURL:      cfg.EmailSiteURL,
		AssetBaseURL: cfg.EmailAssetURL,
	}
}
