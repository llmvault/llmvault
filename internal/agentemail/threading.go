package agentemail

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

// Header returns a case-insensitive RFC header value from Resend's normalized
// header map.
func Header(headers map[string]string, key string) string {
	for actual, value := range headers {
		if strings.EqualFold(strings.TrimSpace(actual), key) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// MessageIDs extracts RFC message identifiers from an In-Reply-To or
// References field. Malformed free text is intentionally ignored.
func MessageIDs(value string) []string {
	var out []string
	for {
		start := strings.IndexByte(value, '<')
		if start < 0 {
			break
		}
		value = value[start:]
		end := strings.IndexByte(value, '>')
		if end < 0 {
			break
		}
		id := strings.TrimSpace(value[:end+1])
		if len(id) > 2 {
			out = append(out, id)
		}
		value = value[end+1:]
	}
	return out
}

func NewReplyToken() (string, error) {
	buf := make([]byte, 15)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)), nil
}

// ReplyLocalPart is deliberately opaque. It is a routing hint, never an auth
// credential; all inbound email remains untrusted.
func ReplyLocalPart(token string) string { return "reply-" + strings.ToLower(strings.TrimSpace(token)) }
