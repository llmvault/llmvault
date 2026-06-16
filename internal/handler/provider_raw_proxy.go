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

type rawProviderProxyContext struct {
	Provider      string
	OrgID         uuid.UUID
	CallerAgentID uuid.UUID
	AgentID       uuid.UUID
	ConnectionID  uuid.UUID
	Method        string
	Path          string
	StatusCode    int
}

type RawProviderProxyHandler struct {
	db            *gorm.DB
	encKey        *crypto.SymmetricKey
	nango         *nango.Client
	provider      string
	allowedPrefix string
}

func NewVercelProxyHandler(db *gorm.DB, encKey *crypto.SymmetricKey, nangoClient *nango.Client) *RawProviderProxyHandler {
	return &RawProviderProxyHandler{db: db, encKey: encKey, nango: nangoClient, provider: "vercel", allowedPrefix: "/"}
}

func NewGlitchTipProxyHandler(db *gorm.DB, encKey *crypto.SymmetricKey, nangoClient *nango.Client) *RawProviderProxyHandler {
	return &RawProviderProxyHandler{db: db, encKey: encKey, nango: nangoClient, provider: "glitchtip", allowedPrefix: "/api/0/"}
}

func NewSlackProxyHandler(db *gorm.DB, encKey *crypto.SymmetricKey, nangoClient *nango.Client) *RawProviderProxyHandler {
	return &RawProviderProxyHandler{db: db, encKey: encKey, nango: nangoClient, provider: "slack", allowedPrefix: "/"}
}

func (h *RawProviderProxyHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agentID, path, ok := h.parseRequest(w, r)
	if !ok {
		return
	}
	eventCtx := rawProviderProxyContext{
		Provider:      h.provider,
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

	owningAgent, err := h.resolveOwningAgent(ctx, *agent.OrgID, agent)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			h.captureProxyFailure(ctx, eventCtx, http.StatusNotFound, "agent is not attached to an agent")
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent is not attached to an agent"})
			return
		}
		h.captureProxyFailure(ctx, eventCtx, http.StatusInternalServerError, "failed to resolve agent")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve agent"})
		return
	}
	eventCtx.AgentID = owningAgent.ID

	conn, providerConfigKey, err := h.resolveConnection(ctx, owningAgent)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			h.captureProxyFailure(ctx, eventCtx, http.StatusNotFound, "no provider connection for org")
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no " + h.provider + " connection for org"})
			return
		}
		h.captureProxyFailure(ctx, eventCtx, http.StatusInternalServerError, "failed to look up provider connection")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up " + h.provider + " connection"})
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
		Provider:      h.provider,
		OrgID:         eventCtx.OrgID,
		CallerAgentID: eventCtx.CallerAgentID,
		AgentID:       eventCtx.AgentID,
		ConnectionID:  eventCtx.ConnectionID,
		Method:        eventCtx.Method,
		Path:          eventCtx.Path,
	}, body) {
		return
	}

	forwardPath := h.nangoProxyPath(path)
	resp, err := h.nango.RawProxyRequestWithHeaders(ctx, r.Method, providerConfigKey, conn.NangoConnectionID, forwardPath, r.URL.RawQuery, proxyRequestBodyFromBytes(r.Method, body), rawProviderProxyHeaders(r))
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, h.provider+"-proxy: nango proxy failed",
			"agent_id", agentID,
			"owner_agent_id", agent.ID,
			"connection_id", conn.ID,
			"path", path,
			"method", r.Method,
			"error", err,
		)
		h.captureProxyFailure(ctx, eventCtx, http.StatusBadGateway, "nango proxy failed")
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": h.provider + " proxy request failed"})
		return
	}

	eventCtx.StatusCode = resp.StatusCode
	if resp.StatusCode >= http.StatusBadRequest {
		h.captureProxyFailure(ctx, eventCtx, resp.StatusCode, h.provider+" upstream returned error")
	}
	copyProxyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(resp.Body)
}

func (h *RawProviderProxyHandler) parseRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, string, bool) {
	if !rawProviderProxyMethodAllowed(r.Method) {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": h.provider + " proxy method not allowed"})
		return uuid.Nil, "", false
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
		return uuid.Nil, "", false
	}
	path := "/" + strings.TrimLeft(chi.URLParam(r, "*"), "/")
	if path == "/" || !strings.HasPrefix(path, h.allowedPrefix) {
		h.captureProxyFailure(r.Context(), rawProviderProxyContext{Provider: h.provider, CallerAgentID: agentID, Method: r.Method, Path: path}, http.StatusBadRequest, "invalid provider api path")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": h.provider + " proxy path is not allowed"})
		return uuid.Nil, "", false
	}
	if h.provider == "slack" && !slackProxyAPIPathAllowed(path) {
		h.captureProxyFailure(r.Context(), rawProviderProxyContext{Provider: h.provider, CallerAgentID: agentID, Method: r.Method, Path: path}, http.StatusBadRequest, "invalid slack api path")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": h.provider + " proxy path is not allowed"})
		return uuid.Nil, "", false
	}
	return agentID, path, true
}

func (h *RawProviderProxyHandler) authenticatedSandbox(ctx context.Context, agentID uuid.UUID, bearerToken string) bool {
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

func (h *RawProviderProxyHandler) resolveOwningAgent(ctx context.Context, orgID uuid.UUID, agent model.Agent) (model.Agent, error) {
	if agent.OrgID != nil && *agent.OrgID == orgID {
		return agent, nil
	}
	var owningAgent model.Agent
	if err := h.db.WithContext(ctx).
		Where("org_id = ? AND status <> ?", orgID, "archived").
		Order("created_at ASC").
		First(&owningAgent).Error; err != nil {
		return model.Agent{}, err
	}
	return owningAgent, nil
}

func (h *RawProviderProxyHandler) resolveConnection(ctx context.Context, agent model.Agent) (model.Connection, string, error) {
	if agent.OrgID == nil {
		return model.Connection{}, "", gorm.ErrRecordNotFound
	}
	result, err := connectionaccess.ResolveAgentProvider(ctx, h.db, *agent.OrgID, agent.ID, h.provider)
	if err != nil {
		return model.Connection{}, "", err
	}
	return result.Connection, result.ProviderConfigKey, nil
}

func (h *RawProviderProxyHandler) captureProxyFailure(ctx context.Context, eventCtx rawProviderProxyContext, status int, reason string) {
	hub := sentrygo.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentrygo.CurrentHub()
	}
	hub.WithScope(func(scope *sentrygo.Scope) {
		scope.SetTag("provider_proxy", eventCtx.Provider)
		scope.SetTag("http.method", eventCtx.Method)
		scope.SetTag("http.status_code", fmt.Sprintf("%d", status))
		if eventCtx.Path != "" {
			scope.SetTag("provider.path", eventCtx.Path)
		}
		if eventCtx.OrgID != uuid.Nil {
			scope.SetTag("org_id", eventCtx.OrgID.String())
		}
		if eventCtx.CallerAgentID != uuid.Nil {
			scope.SetTag("caller_agent_id", eventCtx.CallerAgentID.String())
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
		hub.CaptureException(fmt.Errorf("%s proxy %d: %s", eventCtx.Provider, status, reason))
	})
}

func rawProviderProxyHeaders(r *http.Request) map[string]string {
	headers := map[string]string{}
	for _, key := range []string{"Accept", "Content-Type"} {
		if val := r.Header.Get(key); val != "" {
			headers[key] = val
		}
	}
	return headers
}
