package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/connectionaccess"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

const (
	bugsinkProvider           = "bugsink"
	bugsinkCanonicalAPIPrefix = "/api/canonical/0/"
)

type bugsinkProxyContext struct {
	OrgID         uuid.UUID
	CallerAgentID uuid.UUID
	AgentID       uuid.UUID
	ConnectionID  uuid.UUID
	Method        string
	Path          string
	StatusCode    int
}

type BugsinkProxyHandler struct {
	db     *gorm.DB
	encKey *crypto.SymmetricKey
	nango  *nango.Client
}

func NewBugsinkProxyHandler(db *gorm.DB, encKey *crypto.SymmetricKey, nangoClient *nango.Client) *BugsinkProxyHandler {
	return &BugsinkProxyHandler{db: db, encKey: encKey, nango: nangoClient}
}

func (h *BugsinkProxyHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agentID, path, forwardPath, ok := h.parseRequest(w, r)
	if !ok {
		return
	}
	eventCtx := bugsinkProxyContext{
		CallerAgentID: agentID,
		Method:        r.Method,
		Path:          path,
	}

	bearerToken := extractBearerToken(r)
	if bearerToken == "" {
		h.captureProxyFailure(ctx, eventCtx, http.StatusUnauthorized, "missing authorization")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return
	}

	var agent model.Agent
	if err := h.db.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			h.captureProxyFailure(ctx, eventCtx, http.StatusNotFound, "agent not found")
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		h.captureProxyFailure(ctx, eventCtx, http.StatusInternalServerError, "failed to look up agent")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up agent"})
		return
	}
	if agent.OrgID == nil {
		h.captureProxyFailure(ctx, eventCtx, http.StatusNotFound, "agent has no org")
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent has no org"})
		return
	}
	eventCtx.OrgID = *agent.OrgID

	if !h.authenticatedSandbox(ctx, agentID, bearerToken) {
		h.captureProxyFailure(ctx, eventCtx, http.StatusUnauthorized, "invalid credentials")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	eventCtx.AgentID = agent.ID

	conn, providerConfigKey, err := h.resolveAttachedBugsinkConnection(ctx, agent)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			h.captureProxyFailure(ctx, eventCtx, http.StatusNotFound, "no bugsink connection attached to agent")
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no bugsink connection attached to agent"})
			return
		}
		h.captureProxyFailure(ctx, eventCtx, http.StatusInternalServerError, "failed to look up bugsink connection")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up bugsink connection"})
		return
	}
	eventCtx.ConnectionID = conn.ID

	body, err := readProxyBody(r)
	if err != nil {
		h.captureProxyFailure(ctx, eventCtx, http.StatusBadRequest, "failed to read request body")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}
	if enforceProviderProxyPolicy(w, ctx, providerProxyPolicyContext{
		Provider:      bugsinkProvider,
		OrgID:         eventCtx.OrgID,
		CallerAgentID: eventCtx.CallerAgentID,
		AgentID:       eventCtx.AgentID,
		ConnectionID:  eventCtx.ConnectionID,
		Method:        eventCtx.Method,
		Path:          eventCtx.Path,
	}, body) {
		return
	}

	resp, err := h.nango.RawProxyRequest(ctx, r.Method, providerConfigKey, conn.NangoConnectionID, forwardPath, r.URL.RawQuery, proxyRequestBodyFromBytes(r.Method, body), r.Header.Get("Content-Type"))
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "bugsink-proxy: nango proxy failed",
			"agent_id", agentID,
			"agent_id", agent.ID,
			"connection_id", conn.ID,
			"path", path,
			"method", r.Method,
			"error", err,
		)
		h.captureProxyFailure(ctx, eventCtx, http.StatusBadGateway, "nango proxy failed")
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "bugsink proxy request failed"})
		return
	}

	eventCtx.StatusCode = resp.StatusCode
	if resp.StatusCode >= http.StatusBadRequest {
		h.captureProxyFailure(ctx, eventCtx, resp.StatusCode, "bugsink upstream returned error")
	}
	copyProxyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(resp.Body)
}

func (h *BugsinkProxyHandler) parseRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, string, string, bool) {
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
		return uuid.Nil, "", "", false
	}
	path := "/" + strings.TrimLeft(chi.URLParam(r, "*"), "/")
	if !strings.HasPrefix(path, bugsinkCanonicalAPIPrefix) {
		h.captureProxyFailure(r.Context(), bugsinkProxyContext{CallerAgentID: agentID, Method: r.Method, Path: path}, http.StatusBadRequest, "invalid bugsink api path")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bugsink proxy only supports canonical api paths"})
		return uuid.Nil, "", "", false
	}
	forwardPath := "/" + strings.TrimLeft(strings.TrimPrefix(path, bugsinkCanonicalAPIPrefix), "/")
	return agentID, path, forwardPath, true
}

func (h *BugsinkProxyHandler) authenticatedSandbox(ctx context.Context, agentID uuid.UUID, bearerToken string) bool {
	var sandboxes []model.Sandbox
	if err := h.db.WithContext(ctx).Where("agent_id = ?", agentID).Find(&sandboxes).Error; err != nil {
		return false
	}
	for _, sb := range sandboxes {
		decryptedKey, err := h.encKey.DecryptString(sb.EncryptedRuntimeSecret)
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(bearerToken), []byte(decryptedKey)) == 1 {
			return true
		}
	}
	return false
}

func (h *BugsinkProxyHandler) resolveAttachedBugsinkConnection(ctx context.Context, agent model.Agent) (model.Connection, string, error) {
	if agent.OrgID == nil {
		return model.Connection{}, "", gorm.ErrRecordNotFound
	}
	result, err := connectionaccess.ResolveAgentProvider(ctx, h.db, *agent.OrgID, agent.ID, bugsinkProvider)
	if err != nil {
		return model.Connection{}, "", err
	}
	return result.Connection, result.ProviderConfigKey, nil
}

func (h *BugsinkProxyHandler) captureProxyFailure(ctx context.Context, eventCtx bugsinkProxyContext, status int, reason string) {
	hub := sentrygo.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentrygo.CurrentHub()
	}
	hub.WithScope(func(scope *sentrygo.Scope) {
		scope.SetTag("bugsink_proxy", "true")
		scope.SetTag("http.method", eventCtx.Method)
		scope.SetTag("http.status_code", fmt.Sprintf("%d", status))
		if eventCtx.Path != "" {
			scope.SetTag("bugsink.path", eventCtx.Path)
		}
		if eventCtx.OrgID != uuid.Nil {
			scope.SetTag("org_id", eventCtx.OrgID.String())
		}
		if eventCtx.CallerAgentID != uuid.Nil {
			scope.SetTag("agent_id", eventCtx.CallerAgentID.String())
		}
		if eventCtx.AgentID != uuid.Nil {
			scope.SetTag("agent_id", eventCtx.AgentID.String())
		}
		if eventCtx.ConnectionID != uuid.Nil {
			scope.SetTag("connection_id", eventCtx.ConnectionID.String())
		}
		if status >= http.StatusInternalServerError {
			scope.SetLevel(sentrygo.LevelError)
		} else {
			scope.SetLevel(sentrygo.LevelWarning)
		}
		hub.CaptureException(fmt.Errorf("bugsink proxy %d: %s", status, reason))
	})
}

func copyProxyHeaders(dst, src http.Header) {
	for key, vals := range src {
		if !safeProxyResponseHeader(key) {
			continue
		}
		for _, val := range vals {
			dst.Add(key, val)
		}
	}
}

func safeProxyResponseHeader(key string) bool {
	switch strings.ToLower(key) {
	case "content-type", "content-disposition", "link", "retry-after", "cache-control":
		return true
	}
	lower := strings.ToLower(key)
	return strings.HasPrefix(lower, "x-ratelimit-") || strings.HasPrefix(lower, "ratelimit-")
}
