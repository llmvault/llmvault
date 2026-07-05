package email

import (
	"context"
	"fmt"
	"os"

	"github.com/usehivy/hivy/internal/logging"
)

// FileSender renders emails and writes them to a temp file instead of sending,
// logging the file path. It is the default when no SMTP host is configured, so
// local development can inspect confirmation/reset emails without a mail provider.
type FileSender struct {
	SiteURL      string
	AssetBaseURL string
}

// Send writes a raw ad-hoc email to a temp file.
func (s *FileSender) Send(ctx context.Context, msg Message) error {
	return s.write(ctx, msg.To, msg.Subject, msg.Body)
}

// SendTemplate renders the template and writes the HTML body to a temp file.
func (s *FileSender) SendTemplate(ctx context.Context, msg TemplateMessage) error {
	rendered, err := Render(msg.Slug, msg.Variables, s.SiteURL, s.AssetBaseURL)
	if err != nil {
		return err
	}
	return s.write(ctx, msg.To, rendered.Subject, rendered.HTML)
}

func (s *FileSender) write(ctx context.Context, to, subject, body string) error {
	f, err := os.CreateTemp("", "hivy-email-*.html")
	if err != nil {
		return fmt.Errorf("email: create temp file: %w", err)
	}
	defer f.Close()

	header := fmt.Sprintf("<!-- To: %s | Subject: %s -->\n", to, subject)
	if _, err := f.WriteString(header + body); err != nil {
		return fmt.Errorf("email: write temp file: %w", err)
	}
	logging.FromContext(ctx).InfoContext(ctx, "email written to file (SMTP not configured)",
		"to", to, "subject", subject, "file", f.Name())
	return nil
}
