package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/connectionaccess"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

const (
	gitTokenCacheSize       = 100000
	gitTokenCacheExpirySkew = 10 * time.Minute
)

type gitTokenCacheKey struct {
	agentID           uuid.UUID
	providerConfigKey string
	nangoConnectionID string
}

type gitTokenEntry struct {
	token      string
	validUntil time.Time
}

type gitHubTokenConnection struct {
	conn              model.Connection
	providerConfigKey string
	cacheKey          gitTokenCacheKey
}

// GitCredentialsHandler serves git credential helper requests from sandboxes.
// Sandboxes call this endpoint to get a fresh GitHub token rather than storing
// tokens on disk. Responses follow the git credential helper protocol.
type GitCredentialsHandler struct {
	db     *gorm.DB
	encKey *crypto.SymmetricKey
	nango  *nango.Client
	cache  *lru.Cache[gitTokenCacheKey, *gitTokenEntry]
}

// NewGitCredentialsHandler creates a git credentials handler with an in-memory
// token cache bounded by size. Each entry carries its own Nango-derived expiry.
func NewGitCredentialsHandler(db *gorm.DB, encKey *crypto.SymmetricKey, nangoClient *nango.Client) *GitCredentialsHandler {
	cache, err := lru.New[gitTokenCacheKey, *gitTokenEntry](gitTokenCacheSize)
	if err != nil {
		panic(fmt.Sprintf("create git token cache: %v", err))
	}

	return &GitCredentialsHandler{
		db:     db,
		encKey: encKey,
		nango:  nangoClient,
		cache:  cache,
	}
}

// Handle processes POST /internal/git-credentials/{agentID}.
// Authenticates via the sandbox's runtime secret, then returns a fresh
// GitHub installation token from Nango in git credential protocol format.
func (h *GitCredentialsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	agentIDStr := chi.URLParam(r, "agentID")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
		return
	}

	bearerToken := extractBearerToken(r)
	if bearerToken == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return
	}

	var agent model.Agent
	if err := h.db.Where("id = ?", agentID).First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up agent"})
		return
	}

	if _, ok := resolveSandboxBySecret(h.db, h.encKey, agentID, bearerToken); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	if agent.OrgID == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent has no org"})
		return
	}
	orgID := *agent.OrgID

	tokenConn, err := h.resolveGitHubTokenConnection(r.Context(), orgID, agent)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no github connection for org"})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "git-credentials: failed to resolve github connection",
			"agent_id", agentID,
			"error", err,
		)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up connection"})
		return
	}

	if entry, ok := h.cache.Get(tokenConn.cacheKey); ok {
		if time.Now().Before(entry.validUntil) {
			writeGitCredentials(w, entry.token)
			return
		}
		h.cache.Remove(tokenConn.cacheKey)
	}

	nangoConn, err := h.nango.GetConnection(r.Context(), tokenConn.conn.NangoConnectionID, tokenConn.providerConfigKey)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "git-credentials: failed to fetch from nango",
			"agent_id", agentID,
			"connection_id", tokenConn.conn.ID,
			"error", err,
		)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to fetch github token"})
		return
	}

	creds, ok := nangoConn["credentials"].(map[string]any)
	if !ok {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "no credentials in github-app response"})
		return
	}
	accessToken, ok := creds["access_token"].(string)
	if !ok || accessToken == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "no access_token in github-app credentials"})
		return
	}

	if expiresAt, ok := parseGitTokenExpiresAt(creds); ok {
		validUntil := expiresAt.Add(-gitTokenCacheExpirySkew)
		if time.Now().Before(validUntil) {
			h.cache.Add(tokenConn.cacheKey, &gitTokenEntry{
				token:      accessToken,
				validUntil: validUntil,
			})
		}
	}

	writeGitCredentials(w, accessToken)
}

func (h *GitCredentialsHandler) resolveGitHubTokenConnection(ctx context.Context, orgID uuid.UUID, agent model.Agent) (gitHubTokenConnection, error) {
	result, err := connectionaccess.ResolveAgentProviderAny(ctx, h.db, orgID, agent.ID, "github-app", "github-app-code-reviews")
	if err != nil {
		return gitHubTokenConnection{}, err
	}
	return gitHubTokenConnection{
		conn:              result.Connection,
		providerConfigKey: result.ProviderConfigKey,
		cacheKey:          gitTokenCacheKey{agentID: agent.ID, providerConfigKey: result.ProviderConfigKey, nangoConnectionID: result.Connection.NangoConnectionID},
	}, nil
}

func parseGitTokenExpiresAt(creds map[string]any) (time.Time, bool) {
	raw, ok := creds["expires_at"].(string)
	if !ok || raw == "" {
		return time.Time{}, false
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false
	}
	return expiresAt, true
}

// writeGitCredentials writes a response in git credential helper protocol format.
func writeGitCredentials(w http.ResponseWriter, token string) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "username=x-access-token\npassword=%s\n", token)
}

// extractBearerToken extracts the token from an "Authorization: Bearer {token}" header.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}
