package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/counter"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/mcpserver"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

// MCPHandler handles MCP protocol requests for scoped proxy tokens.
type MCPHandler struct {
	db                *gorm.DB
	signingKey        []byte
	catalog           *catalog.Catalog
	nango             *nango.Client
	counter           *counter.Counter
	webTools          mcpserver.WebToolsFunc
	knowledgeTools    mcpserver.KnowledgeToolsFunc
	memoryTools       mcpserver.MemoryToolsFunc
	imageTools        mcpserver.ImageGenerationToolsFunc
	skillTools        mcpserver.SkillToolsFunc
	agentBuilderTools mcpserver.AgentBuilderToolsFunc
	sheetTools        mcpserver.SheetToolsFunc
	appsTools         mcpserver.AppsToolsFunc
	ServerCache       *mcpserver.ServerCache
}

// NewMCPHandler creates a new MCP handler.
func NewMCPHandler(db *gorm.DB, signingKey []byte, cat *catalog.Catalog, nangoClient *nango.Client, ctr *counter.Counter) *MCPHandler {
	return &MCPHandler{
		db:          db,
		signingKey:  signingKey,
		catalog:     cat,
		nango:       nangoClient,
		counter:     ctr,
		ServerCache: mcpserver.NewServerCache(),
	}
}

// SetWebTools sets the callback for registering web_fetch and web_search
// on MCP servers.
func (h *MCPHandler) SetWebTools(fn mcpserver.WebToolsFunc) {
	h.webTools = fn
}

// SetKnowledgeTools sets the callback for registering knowledge-base tools.
func (h *MCPHandler) SetKnowledgeTools(fn mcpserver.KnowledgeToolsFunc) {
	h.knowledgeTools = fn
}

// SetMemoryTools sets the callback for registering agent memory tools.
func (h *MCPHandler) SetMemoryTools(fn mcpserver.MemoryToolsFunc) {
	h.memoryTools = fn
}

// SetImageGenerationTools sets the callback for registering image generation tools.
func (h *MCPHandler) SetImageGenerationTools(fn mcpserver.ImageGenerationToolsFunc) {
	h.imageTools = fn
}

// SetSkillTools sets the callback for registering the read-only skill tools.
func (h *MCPHandler) SetSkillTools(fn mcpserver.SkillToolsFunc) {
	h.skillTools = fn
}

// SetAgentBuilderTools sets the callback for registering the agent-builder
// tools (list_org_plugins, create_agent, update_agent).
func (h *MCPHandler) SetAgentBuilderTools(fn mcpserver.AgentBuilderToolsFunc) {
	h.agentBuilderTools = fn
}

// SetSheetTools sets the callback for registering the plugin-gated sheets
// tool group.
func (h *MCPHandler) SetSheetTools(fn mcpserver.SheetToolsFunc) {
	h.sheetTools = fn
}

// SetAppsTools sets the callback for registering the plugin-gated apps
// tool group.
func (h *MCPHandler) SetAppsTools(fn mcpserver.AppsToolsFunc) {
	h.appsTools = fn
}

// StreamableHTTPHandler returns an HTTP handler for the MCP Streamable HTTP transport.
func (h *MCPHandler) StreamableHTTPHandler() http.Handler {
	return mcp.NewStreamableHTTPHandler(h.serverFactory, &mcp.StreamableHTTPOptions{
		Stateless:                  true,
		Logger:                     slog.Default(),
		DisableLocalhostProtection: true,
	})
}

// SSEHandler returns an HTTP handler for the MCP SSE transport.
func (h *MCPHandler) SSEHandler() http.Handler {
	return mcp.NewSSEHandler(h.serverFactory, nil)
}

// serverFactory returns or builds an MCP server for the given request's token.
func (h *MCPHandler) serverFactory(r *http.Request) *mcp.Server {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "mcp: no claims in context")
		return nil
	}

	ctx := r.Context()
	srv, err := h.ServerCache.GetOrBuild(claims.JTI, func() (*mcp.Server, time.Time, error) {

		var token model.Token
		if err := h.db.WithContext(ctx).Where("jti = ?", claims.JTI).First(&token).Error; err != nil {
			return nil, time.Time{}, err
		}

		toolFilter := h.agentProxyMCPToolFilter(ctx, &token)
		srv, err := mcpserver.BuildServer(ctx, &token, h.db, h.counter, h.webTools, h.knowledgeTools, h.memoryTools, h.imageTools, h.skillTools, h.agentBuilderTools, h.sheetTools, h.appsTools, toolFilter)
		if err != nil {
			return nil, time.Time{}, err
		}

		return srv, token.ExpiresAt, nil
	})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "mcp: failed to build server", "error", err, "jti", claims.JTI)
		return nil
	}

	return srv
}

// agentProxyMCPToolFilter resolves the exact compiler policy before the server
// is cached for a token JTI. That means tools outside the allow-list are absent
// from the MCP tools/list response and never enter the runtime's registry.
// Failed agent resolution is fail-closed for agent proxy tokens.
func (h *MCPHandler) agentProxyMCPToolFilter(ctx context.Context, token *model.Token) *model.ToolFilter {
	if token == nil || token.Meta == nil {
		return nil
	}
	tokenType, _ := token.Meta[model.TokenMetaType].(string)
	if tokenType != model.TokenTypeAgentProxy {
		return nil
	}
	if h == nil || h.db == nil {
		return agentruntime.ResolveAgentMCPToolFilter(ctx, nil, nil)
	}
	rawAgentID, _ := token.Meta[model.TokenMetaAgentID].(string)
	agentID, err := uuid.Parse(strings.TrimSpace(rawAgentID))
	if err != nil || agentID == uuid.Nil {
		return agentruntime.ResolveAgentMCPToolFilter(ctx, nil, nil)
	}
	var agent model.Agent
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, token.OrgID, "archived").
		First(&agent).Error; err != nil {
		return agentruntime.ResolveAgentMCPToolFilter(ctx, nil, nil)
	}
	return agentruntime.ResolveAgentMCPToolFilter(ctx, h.db, &agent)
}

// ValidateJTIMatch is middleware ensuring the URL {jti} matches the JWT's JTI claim.
func (h *MCPHandler) ValidateJTIMatch(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlJTI := chi.URLParam(r, "jti")
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "token JTI does not match URL"})
			return
		}
		if urlJTI == claims.JTI {
			next.ServeHTTP(w, r)
			return
		}
		if !h.allowSameAgentSandboxProxyToken(r, urlJTI, claims.JTI) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "token JTI does not match URL"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *MCPHandler) allowSameAgentSandboxProxyToken(r *http.Request, urlJTI, bearerJTI string) bool {
	if h == nil || h.db == nil || urlJTI == "" || bearerJTI == "" {
		return false
	}
	var tokens []model.Token
	if err := h.db.WithContext(r.Context()).
		Where("jti IN ?", []string{urlJTI, bearerJTI}).
		Find(&tokens).Error; err != nil || len(tokens) != 2 {
		return false
	}
	var urlToken, bearerToken *model.Token
	for i := range tokens {
		switch tokens[i].JTI {
		case urlJTI:
			urlToken = &tokens[i]
		case bearerJTI:
			bearerToken = &tokens[i]
		}
	}
	if urlToken == nil || bearerToken == nil {
		return false
	}
	now := time.Now().UTC()
	if !activeAgentProxyToken(urlToken, now) || !activeAgentProxyToken(bearerToken, now) {
		return false
	}
	if urlToken.OrgID != bearerToken.OrgID {
		return false
	}
	urlAgentID, _ := urlToken.Meta[model.TokenMetaAgentID].(string)
	bearerAgentID, _ := bearerToken.Meta[model.TokenMetaAgentID].(string)
	urlSandboxID, _ := urlToken.Meta[model.TokenMetaSandboxID].(string)
	bearerSandboxID, _ := bearerToken.Meta[model.TokenMetaSandboxID].(string)
	return urlAgentID != "" &&
		urlAgentID == bearerAgentID &&
		urlSandboxID != "" &&
		urlSandboxID == bearerSandboxID
}

func activeAgentProxyToken(token *model.Token, now time.Time) bool {
	if token == nil || token.RevokedAt != nil || !token.ExpiresAt.After(now) {
		return false
	}
	tokenType, _ := token.Meta[model.TokenMetaType].(string)
	return tokenType == model.TokenTypeAgentProxy
}

// ValidateHasScopes preserves the existing middleware hook while MCP no longer
// uses integration connection scopes. Agent proxy tokens are the runtime path.
func (h *MCPHandler) ValidateHasScopes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing claims"})
			return
		}

		var token model.Token
		if err := h.db.Where("jti = ?", claims.JTI).First(&token).Error; err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token not found"})
			return
		}
		if token.Meta != nil {
			if tokenType, _ := token.Meta["type"].(string); tokenType == "agent_proxy" {
				next.ServeHTTP(w, r)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
