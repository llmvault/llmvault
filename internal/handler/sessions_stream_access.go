package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// SandboxAccess handles POST /v1/sessions/{id}/sandbox-access.
// @Summary Mint sandbox repository access
// @Description Returns the sandbox base URL and a short-lived JWT scoped to read-only repository APIs.
// @Tags sessions
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} sessionSandboxAccessResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sessions/{id}/sandbox-access [post]
func (h *SessionHandler) SandboxAccess(w http.ResponseWriter, r *http.Request) {
	session, userID, ok := h.authorizeSession(w, r, true)
	if !ok {
		return
	}
	if h.runtimeEncKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "runtime sandbox access is not configured"})
		return
	}
	sb, err := h.sessionSandboxForAccess(r.Context(), &session)
	if err != nil {
		h.writeSessionSandboxWakeError(w, err)
		return
	}
	runtimeSecret, err := h.runtimeEncKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "runtime sandbox access is not available"})
		return
	}
	expiresAt := time.Now().UTC().Add(time.Hour)
	scopes := []string{"repo:read"}
	sub := ""
	if userID != nil {
		sub = userID.String()
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":        "hivy",
		"aud":        "hivy-runtime",
		"sub":        sub,
		"org_id":     session.OrgID.String(),
		"session_id": session.ID.String(),
		"sandbox_id": sb.ID.String(),
		"scopes":     scopes,
		"iat":        time.Now().UTC().Unix(),
		"nbf":        time.Now().UTC().Add(-5 * time.Second).Unix(),
		"exp":        expiresAt.Unix(),
		"jti":        uuid.NewString(),
	})
	signed, err := token.SignedString([]byte(runtimeSecret))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to mint sandbox access"})
		return
	}
	writeJSON(w, http.StatusOK, sessionSandboxAccessResponse{
		SessionID:      session.ID.String(),
		SandboxID:      sb.ID.String(),
		SandboxBaseURL: strings.TrimRight(sb.RuntimeURL, "/"),
		Token:          signed,
		ExpiresAt:      expiresAt.Format(time.RFC3339),
		Scopes:         scopes,
	})
}

func (h *SessionHandler) sessionSandboxForAccess(ctx context.Context, session *model.Session) (*model.Sandbox, error) {
	sb, err := h.loadSessionSandbox(ctx, session)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(sb.RuntimeURL) == "" {
		return nil, errSessionSandboxUnavailable
	}
	return sb, nil
}
