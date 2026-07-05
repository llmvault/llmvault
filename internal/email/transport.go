package email

import "context"

// TransportConfig configures the worker-side email transport.
type TransportConfig struct {
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPTLS      string
	From         string
	SiteURL      string
	AssetBaseURL string
}

// NewTransport builds the transport that actually delivers email: an SMTP sender
// when a host is configured, otherwise a FileSender that renders each email to a
// temp file (logging the path) so local dev works without a mail provider.
func NewTransport(c TransportConfig) Sender {
	if c.SMTPHost != "" {
		return NewSMTPSender(SMTPConfig{
			Host:         c.SMTPHost,
			Port:         c.SMTPPort,
			Username:     c.SMTPUsername,
			Password:     c.SMTPPassword,
			TLS:          c.SMTPTLS,
			From:         c.From,
			SiteURL:      c.SiteURL,
			AssetBaseURL: c.AssetBaseURL,
		})
	}
	return &FileSender{SiteURL: c.SiteURL, AssetBaseURL: c.AssetBaseURL}
}

// NoopSender discards emails. Intended for tests that exercise handlers without
// asserting on email delivery.
type NoopSender struct{}

func (NoopSender) Send(context.Context, Message) error                 { return nil }
func (NoopSender) SendTemplate(context.Context, TemplateMessage) error { return nil }
