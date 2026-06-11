package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const webStreamTokenLifetime = time.Hour

// streamTokenHeader is the preferred header name for the signed stream token.
// Clients that can set request headers should use this instead of the ?token=
// query parameter to avoid the token appearing in access logs and Sentry
// breadcrumbs.
const streamTokenHeader = "X-Stream-Token" // #nosec G101 -- HTTP header name, not a credential

func (h *EmployeeHandler) signedWebStreamURL(employeeID, sessionID uuid.UUID, streamID string) (string, error) {
	if strings.TrimSpace(streamID) == "" {
		return "", fmt.Errorf("stream id is required")
	}
	expiresAt := time.Now().Add(webStreamTokenLifetime).Unix()
	token, err := h.signWebStreamToken(sessionID, streamID, expiresAt)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("/v1/employees/%s/sessions/%s/streams/%s?token=%s", employeeID, sessionID, streamID, token), nil
}

// extractWebStreamToken extracts the signed stream token from the request,
// preferring the X-Stream-Token header over the ?token= query parameter so
// that tokens are not captured in access logs or Sentry breadcrumbs.
func extractWebStreamToken(r *http.Request) string {
	if tok := r.Header.Get(streamTokenHeader); tok != "" {
		return tok
	}
	return r.URL.Query().Get("token")
}

func (h *EmployeeHandler) signWebStreamToken(sessionID uuid.UUID, streamID string, expiresAt int64) (string, error) {
	if len(h.compileDeps.SigningKey) == 0 {
		return "", fmt.Errorf("missing signing key")
	}
	payload := fmt.Sprintf("%s|%s|%d", sessionID, streamID, expiresAt)
	mac := hmac.New(sha256.New, h.compileDeps.SigningKey)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (h *EmployeeHandler) verifyWebStreamToken(sessionID uuid.UUID, streamID, token string) bool {
	if len(h.compileDeps.SigningKey) == 0 || strings.TrimSpace(token) == "" {
		return false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	payload := string(payloadBytes)
	fields := strings.Split(payload, "|")
	if len(fields) != 3 || fields[0] != sessionID.String() || fields[1] != streamID {
		return false
	}
	expiresAt, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || time.Now().Unix() > expiresAt {
		return false
	}
	mac := hmac.New(sha256.New, h.compileDeps.SigningKey)
	_, _ = mac.Write([]byte(payload))
	return hmac.Equal(sig, mac.Sum(nil))
}
