package email

import (
	"context"
	"errors"
	"fmt"

	"github.com/wneessen/go-mail"
)

// SMTPConfig configures the SMTP transport. Any SMTP provider works (Resend
// SMTP, SES, Postmark, self-hosted). TLS is one of "starttls" (default), "ssl"
// (implicit TLS, e.g. port 465) or "none".
type SMTPConfig struct {
	Host         string
	Port         int
	Username     string
	Password     string
	TLS          string
	From         string
	SiteURL      string
	AssetBaseURL string
}

// SMTPSender renders templates to HTML/plaintext in-process and delivers them
// over SMTP. It is the worker-side transport (handlers enqueue; the worker sends).
type SMTPSender struct {
	cfg SMTPConfig
}

// NewSMTPSender builds an SMTP-backed Sender.
func NewSMTPSender(cfg SMTPConfig) *SMTPSender {
	return &SMTPSender{cfg: cfg}
}

// Send delivers a raw plaintext ad-hoc email.
func (s *SMTPSender) Send(ctx context.Context, msg Message) error {
	if msg.To == "" {
		return errors.New("smtp: empty recipient")
	}
	if msg.Subject == "" {
		return errors.New("smtp: empty subject")
	}
	m, err := s.newMessage(msg.To, msg.Subject)
	if err != nil {
		return err
	}
	m.SetBodyString(mail.TypeTextPlain, msg.Body)
	return s.deliver(ctx, m)
}

// SendTemplate renders the template and delivers a multipart (text + HTML) email.
func (s *SMTPSender) SendTemplate(ctx context.Context, msg TemplateMessage) error {
	if msg.To == "" {
		return errors.New("smtp: empty recipient")
	}
	if msg.Slug == "" {
		return errors.New("smtp: empty template slug")
	}
	rendered, err := Render(msg.Slug, msg.Variables, s.cfg.SiteURL, s.cfg.AssetBaseURL)
	if err != nil {
		return err
	}
	m, err := s.newMessage(msg.To, rendered.Subject)
	if err != nil {
		return err
	}
	if rendered.Text != "" {
		m.SetBodyString(mail.TypeTextPlain, rendered.Text)
		m.AddAlternativeString(mail.TypeTextHTML, rendered.HTML)
	} else {
		m.SetBodyString(mail.TypeTextHTML, rendered.HTML)
	}
	return s.deliver(ctx, m)
}

func (s *SMTPSender) newMessage(to, subject string) (*mail.Msg, error) {
	m := mail.NewMsg()
	if err := m.From(s.cfg.From); err != nil {
		return nil, fmt.Errorf("smtp: invalid From %q: %w", s.cfg.From, err)
	}
	if err := m.To(to); err != nil {
		return nil, fmt.Errorf("smtp: invalid To %q: %w", to, err)
	}
	m.Subject(subject)
	return m, nil
}

func (s *SMTPSender) deliver(ctx context.Context, m *mail.Msg) error {
	opts := []mail.Option{mail.WithPort(s.cfg.Port)}
	switch s.cfg.TLS {
	case "ssl":
		opts = append(opts, mail.WithSSL())
	case "none":
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	default: // starttls
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	}
	// Only authenticate when a username is configured; some relays accept
	// unauthenticated submission from trusted networks.
	if s.cfg.Username != "" {
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(s.cfg.Username),
			mail.WithPassword(s.cfg.Password),
		)
	}
	client, err := mail.NewClient(s.cfg.Host, opts...)
	if err != nil {
		return fmt.Errorf("smtp: build client: %w", err)
	}
	if err := client.DialAndSendWithContext(ctx, m); err != nil {
		return fmt.Errorf("smtp: send: %w", err)
	}
	return nil
}
