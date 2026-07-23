package proxy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

const (
	routeBlacklistTTL = 24 * time.Hour
	routeProbeTTL     = 30 * time.Second
)

var ErrNoHealthyRoute = errors.New("proxy: no healthy route for model")

// RouteCandidate is one credential-backed upstream option for a canonical
// model. Candidates are ordered by the model catalog's proxy route order.
type RouteCandidate struct {
	CredentialID     string
	ProviderID       string
	UpstreamID       string
	CanonicalModelID string
}

// ModelRouter resolves system-credential-backed proxy routes and maintains
// short-lived, shared provider health state. It intentionally routes only
// platform credentials: BYOK tokens remain bound to their selected credential.
type ModelRouter struct {
	db    *gorm.DB
	redis redis.UniversalClient
	reg   *registry.Registry
	now   func() time.Time
}

func NewModelRouter(db *gorm.DB, redisClient redis.UniversalClient, reg *registry.Registry) *ModelRouter {
	if reg == nil {
		reg = registry.Global()
	}
	return &ModelRouter{db: db, redis: redisClient, reg: reg, now: time.Now}
}

// Candidates returns healthy candidates for a system proxy request. If all
// configured routes are cooling down, the primary route receives one
// distributed half-open probe so a recovered default can rejoin immediately.
func (r *ModelRouter) Candidates(ctx context.Context, claims *middleware.TokenClaims, canonicalModelID string) ([]RouteCandidate, error) {
	if r == nil || r.db == nil || claims == nil || !claims.IsSystem {
		return nil, nil
	}
	canonicalModelID = strings.TrimSpace(canonicalModelID)
	if canonicalModelID == "" {
		return nil, nil
	}
	routes := r.reg.ProxyRoutesForModel(canonicalModelID)
	if len(routes) == 0 {
		return nil, nil
	}

	providerIDs := make([]string, 0, len(routes))
	for _, route := range routes {
		providerIDs = appendUnique(providerIDs, route.ProviderID)
	}

	var credentials []model.Credential
	if err := r.db.WithContext(ctx).
		Where("org_id IS NULL AND revoked_at IS NULL AND provider_id IN ?", providerIDs).
		Order("created_at ASC").
		Find(&credentials).Error; err != nil {
		return nil, fmt.Errorf("list system proxy credentials: %w", err)
	}

	all := candidatesForRoutes(canonicalModelID, routes, credentials)
	if len(all) == 0 {
		return nil, nil
	}
	healthy := make([]RouteCandidate, 0, len(all))
	for _, candidate := range all {
		blocked, err := r.isBlacklisted(ctx, canonicalModelID, candidate.ProviderID)
		if err != nil {
			// Redis is an availability optimisation. A Redis outage must never
			// block inference or cause every request to fail closed.
			return all, nil
		}
		if !blocked {
			healthy = append(healthy, candidate)
		}
	}
	if len(healthy) > 0 {
		return healthy, nil
	}

	if ok, err := r.acquireProbe(ctx, canonicalModelID); err != nil {
		return all, nil
	} else if ok {
		return all[:1], nil
	}
	return nil, ErrNoHealthyRoute
}

func candidatesForRoutes(canonicalModelID string, routes []registry.ModelRoute, credentials []model.Credential) []RouteCandidate {
	out := make([]RouteCandidate, 0, len(routes))
	for _, route := range routes {
		for _, credential := range credentials {
			if credential.ProviderID != route.ProviderID {
				continue
			}
			candidateModelID := route.CanonicalModelID
			if candidateModelID == "" {
				candidateModelID = canonicalModelID
			}
			out = append(out, RouteCandidate{
				CredentialID:     credential.ID.String(),
				ProviderID:       route.ProviderID,
				UpstreamID:       route.ModelID,
				CanonicalModelID: candidateModelID,
			})
			break // Provider health is shared; use its oldest active credential.
		}
	}
	return out
}

func (r *ModelRouter) MarkFailure(ctx context.Context, canonicalModelID string, candidate RouteCandidate) {
	if r == nil || r.redis == nil || canonicalModelID == "" || candidate.ProviderID == "" {
		return
	}
	_ = r.redis.Set(ctx, r.blacklistKey(canonicalModelID, candidate.ProviderID), r.now().UTC().Format(time.RFC3339), routeBlacklistTTL).Err() // best-effort availability state
}

func (r *ModelRouter) MarkSuccess(ctx context.Context, canonicalModelID string, candidate RouteCandidate) {
	if r == nil || r.redis == nil || canonicalModelID == "" || candidate.ProviderID == "" {
		return
	}
	_ = r.redis.Del(ctx, r.blacklistKey(canonicalModelID, candidate.ProviderID)).Err() // best-effort availability state
}

func (r *ModelRouter) isBlacklisted(ctx context.Context, canonicalModelID, providerID string) (bool, error) {
	if r.redis == nil {
		return false, nil
	}
	exists, err := r.redis.Exists(ctx, r.blacklistKey(canonicalModelID, providerID)).Result()
	if err != nil {
		return false, fmt.Errorf("read route blacklist: %w", err)
	}
	return exists > 0, nil
}

func (r *ModelRouter) acquireProbe(ctx context.Context, canonicalModelID string) (bool, error) {
	if r.redis == nil {
		return true, nil
	}
	result, err := r.redis.SetArgs(ctx, r.probeKey(canonicalModelID), "1", redis.SetArgs{Mode: "NX", TTL: routeProbeTTL}).Result()
	if err != nil && err != redis.Nil {
		return false, fmt.Errorf("acquire route probe: %w", err)
	}
	return result == "OK", nil
}

func (r *ModelRouter) blacklistKey(canonicalModelID, providerID string) string {
	return "hivy:llm:route:blacklist:" + canonicalModelID + ":" + providerID
}

func (r *ModelRouter) probeKey(canonicalModelID string) string {
	return "hivy:llm:route:probe:" + canonicalModelID
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
