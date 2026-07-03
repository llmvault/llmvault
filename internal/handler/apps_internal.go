package handler

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sheets"
)

// appActorHeader optionally carries the user UUID an app forwards from its
// cookie session so in-app mutations are attributed to the human who acted.
const appActorHeader = "X-Hivy-App-Actor"

// AppsInternalHandler serves the internal app API
// (/internal/apps/{appID}/v1/*) — the only Hivy surface an app backend can
// reach. Auth is the app's runtime secret as a bearer, mirroring the sandbox
// runtime-secret pattern (UploadsHandler.authAgent). Handlers are thin shells
// over sheets.Service scoped to the app's ONE bound sheet; no schema-mutation
// endpoint exists here by construction (apps plan §1.2).
type AppsInternalHandler struct {
	db        *gorm.DB
	svc       *sheets.Service
	encKey    *crypto.SymmetricKey
	presigner SheetsPresigner
}

func NewAppsInternalHandler(db *gorm.DB, svc *sheets.Service, encKey *crypto.SymmetricKey) *AppsInternalHandler {
	return &AppsInternalHandler{db: db, svc: svc, encKey: encKey}
}

// WithPresigner enables attachment download URLs.
func (h *AppsInternalHandler) WithPresigner(p SheetsPresigner) *AppsInternalHandler {
	h.presigner = p
	return h
}

// authApp resolves the app from the URL param and verifies the bearer matches
// the app's decrypted runtime secret (constant-time). On failure it writes the
// JSON error response and returns false — callers must return.
func (h *AppsInternalHandler) authApp(w http.ResponseWriter, r *http.Request) (*model.App, bool) {
	if h.encKey == nil || h.svc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "app endpoints not configured"})
		return nil, false
	}

	appID, err := uuid.Parse(chi.URLParam(r, "appID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid app_id"})
		return nil, false
	}

	bearer := bearerFromHeader(r.Header.Get("Authorization"))
	if bearer == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return nil, false
	}

	var app model.App
	if err := h.db.WithContext(r.Context()).
		Where("id = ? AND archived_at IS NULL", appID).
		First(&app).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "app not found"})
			return nil, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load app"})
		return nil, false
	}

	wantKey, err := h.encKey.DecryptString(app.EncryptedAppSecret)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "decrypt app secret", "app_id", appID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to verify credentials"})
		return nil, false
	}
	if subtle.ConstantTimeCompare([]byte(bearer), []byte(wantKey)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid app secret"})
		return nil, false
	}

	return &app, true
}

// appPageID parses the pageID path param and enforces that the page belongs to
// the app's bound sheet. A page of any other sheet — same org included — 404s,
// so it is indistinguishable from a missing one.
func (h *AppsInternalHandler) appPageID(w http.ResponseWriter, r *http.Request, app *model.App) (uuid.UUID, bool) {
	pageID, err := uuid.Parse(chi.URLParam(r, "pageID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "pageID must be a uuid"})
		return uuid.Nil, false
	}
	if err := h.svc.PageInSheet(r.Context(), app.OrgID, app.SheetID, pageID); err != nil {
		writeSheetsError(w, r, err)
		return uuid.Nil, false
	}
	return pageID, true
}

// appActor resolves the optional X-Hivy-App-Actor header into a sheets actor.
// Fail-closed like access.Resolve: a present-but-malformed or non-member user
// id is a 403, never silently treated as "no actor". An absent header records
// an app-only mutation attributed to the app's creating agent.
func (h *AppsInternalHandler) appActor(w http.ResponseWriter, r *http.Request, app *model.App) (sheets.Actor, bool) {
	actor := sheets.Actor{ChannelID: app.ChannelID}
	raw := strings.TrimSpace(r.Header.Get(appActorHeader))
	if raw == "" {
		actor.AgentID = app.CreatedByAgentID
		return actor, true
	}
	resolved, err := access.Resolve(r.Context(), h.db, app.OrgID, raw)
	if err != nil {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: err.Error()})
		return sheets.Actor{}, false
	}
	actor.UserID = &resolved.UserID
	return actor, true
}
