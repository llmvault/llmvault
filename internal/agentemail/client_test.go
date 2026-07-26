package agentemail

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientGetReceivedAndSend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}
		switch r.URL.Path {
		case "/emails/receiving/email_123":
			if r.Method != http.MethodGet {
				t.Fatalf("method = %s", r.Method)
			}
			_, _ = io.WriteString(w, `{"id":"email_123","from":"person@example.test","to":["agent@example.test"],"subject":"Hello","text":"hi","message_id":"<m@example.test>","created_at":"2026-07-18T00:00:00Z"}`)
		case "/emails":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s", r.Method)
			}
			if got := r.Header.Get("Idempotency-Key"); got != "agent-email/message-1" {
				t.Fatalf("idempotency key = %q", got)
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"subject":"Reply"`) {
				t.Fatalf("body = %s", body)
			}
			_, _ = io.WriteString(w, `{"id":"email_sent_123"}`)
		case "/emails/email_sent_123":
			if r.Method != http.MethodGet {
				t.Fatalf("method = %s", r.Method)
			}
			// Resend currently returns sent-email timestamps in this
			// PostgreSQL-style format. The client intentionally ignores it.
			_, _ = io.WriteString(w, `{"object":"email","id":"email_sent_123","message_id":"<sent-123@example.test>","created_at":"2026-07-18 00:01:00.123456+00","last_event":"delivered"}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := newClient("test-key", server.URL)
	received, err := client.GetReceived(context.Background(), "email_123")
	if err != nil {
		t.Fatalf("GetReceived: %v", err)
	}
	if received.MessageID != "<m@example.test>" || received.Text == nil || *received.Text != "hi" {
		t.Fatalf("received = %#v", received)
	}
	sent, err := client.Send(context.Background(), SendRequest{From: "agent@example.test", To: []string{"person@example.test"}, Subject: "Reply", Text: "hello"}, "agent-email/message-1")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if sent.ID != "email_sent_123" {
		t.Fatalf("sent = %#v", sent)
	}
	details, err := client.GetSent(context.Background(), sent.ID)
	if err != nil {
		t.Fatalf("GetSent: %v", err)
	}
	if details.MessageID != "<sent-123@example.test>" {
		t.Fatalf("sent details = %#v", details)
	}
}
