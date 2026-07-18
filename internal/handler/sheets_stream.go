package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/sheets"
)

const (
	sheetsLiveTokenTTL = 10 * time.Minute
	sheetsLiveAudience = "hivy-sheets-live"
	sheetsLiveScope    = "sheets:read"
)

type sheetsLiveClaims struct {
	OrgID   string   `json:"org_id"`
	SheetID string   `json:"sheet_id"`
	Scopes  []string `json:"scopes"`
	jwt.RegisteredClaims
}

// LiveToken mints a short-lived, sheet-scoped JWT the browser presents when
// connecting directly to /v1/sheets/{sheetID}/live (bypassing the Next proxy),
// mirroring the canvas preview-token pattern.
// @Summary Mint a sheet live-stream token
// @Description Returns a short-lived JWT for the direct SSE stream. The caller must be able to use the sheet's team.
// @Tags sheets
// @Produce json
// @Param sheetID path string true "Sheet ID"
// @Success 200 {object} sheetLiveTokenResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/live-token [post]
func (h *SheetsHandler) LiveToken(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	if len(h.signingKey) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "sheet live streaming is not configured"})
		return
	}
	sheetID, ok := sheetsPathUUID(w, r, "sheetID")
	if !ok {
		return
	}
	if _, err := h.svc.SheetByID(r.Context(), org.ID, sheetID); err != nil {
		writeSheetsError(w, r, err)
		return
	}
	now := time.Now().UTC()
	expiresAt := now.Add(sheetsLiveTokenTTL)
	claims := sheetsLiveClaims{
		OrgID:   org.ID.String(),
		SheetID: sheetID.String(),
		Scopes:  []string{sheetsLiveScope},
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "hivy",
			Audience:  jwt.ClaimStrings{sheetsLiveAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			ID:        uuid.NewString(),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(h.signingKey)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to mint live token"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"token":      token,
		"expires_at": expiresAt.Format(time.RFC3339),
	})
}

// Live streams a sheet's realtime events over SSE. Auth is the live token
// only — browsers connect directly, so the normal session middleware never
// runs here. Global CORS middleware already covers this route.
// @Summary Stream sheet realtime events (SSE)
// @Description Server-sent-events stream of a sheet's realtime row/field events. Authenticated by the short-lived live token (query param), not the session cookie. Content-Type is text/event-stream.
// @Tags sheets
// @Produce text/event-stream
// @Param sheetID path string true "Sheet ID"
// @Param token query string true "Live stream token"
// @Success 200 {string} string "event stream"
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Router /v1/sheets/{sheetID}/live [get]
func (h *SheetsHandler) Live(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil || h.redis == nil || len(h.signingKey) == 0 {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "sheet live streaming is not configured"})
		return
	}
	sheetID, ok := sheetsPathUUID(w, r, "sheetID")
	if !ok {
		return
	}
	claims, ok := h.authorizeLive(w, r, sheetID)
	if !ok {
		return
	}
	orgID, err := uuid.Parse(claims.OrgID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid live token"})
		return
	}
	if _, err := h.svc.SheetByID(r.Context(), orgID, sheetID); err != nil {
		writeSheetsError(w, r, err)
		return
	}

	ctx := r.Context()
	pubsub := h.redis.Subscribe(ctx, sheets.LiveChannel(sheetID))
	defer pubsub.Close()
	if _, err := pubsub.Receive(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "failed to subscribe to sheet stream"})
		return
	}
	liveCh := pubsub.Channel(redis.WithChannelSize(256))

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	if err := writeSessionSSE(rc, w, "sheet.control", "", map[string]any{
		"type":     "ready",
		"sheet_id": sheetID.String(),
	}); err != nil {
		return
	}

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()
	deadline := time.NewTimer(time.Until(claims.ExpiresAt.Time))
	defer deadline.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			_ = writeSessionSSE(rc, w, "sheet.control", "", map[string]any{"type": "token_expired"})
			return
		case msg, chOk := <-liveCh:
			if !chOk {
				_ = writeSessionSSE(rc, w, "sheet.control", "", map[string]any{"type": "resync"})
				return
			}
			if err := writeSessionSSE(rc, w, "sheet.event", "", json.RawMessage(msg.Payload)); err != nil {
				return
			}
		case <-keepalive.C:
			if err := writeSessionSSE(rc, w, "sheet.control", "", map[string]any{
				"type":      "keepalive",
				"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
			}); err != nil {
				return
			}
		}
	}
}

// authorizeLive validates the bearer (or ?token=) live JWT: HS256 signature,
// audience, scope, and that the token was minted for this exact sheet.
func (h *SheetsHandler) authorizeLive(w http.ResponseWriter, r *http.Request, sheetID uuid.UUID) (*sheetsLiveClaims, bool) {
	raw := sheetsLiveTokenFromRequest(r)
	if raw == "" {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing live token"})
		return nil, false
	}
	claims := &sheetsLiveClaims{}
	parsed, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return h.signingKey, nil
	}, jwt.WithAudience(sheetsLiveAudience), jwt.WithIssuer("hivy"))
	if err != nil || !parsed.Valid {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid or expired live token"})
		return nil, false
	}
	scoped := false
	for _, scope := range claims.Scopes {
		if scope == sheetsLiveScope {
			scoped = true
		}
	}
	if !scoped || claims.ExpiresAt == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid or expired live token"})
		return nil, false
	}
	if claims.SheetID != sheetID.String() {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "live token does not grant access to this sheet"})
		return nil, false
	}
	return claims, true
}

func sheetsLiveTokenFromRequest(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}
