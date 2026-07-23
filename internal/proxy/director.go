package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/observe"
	"github.com/usehivy/hivy/internal/providerheaders"
)

type routePlanContextKey struct{}

type routePlan struct {
	canonicalModel  string
	candidates      []RouteCandidate
	index           int
	streaming       bool
	originalPath    string
	originalRawPath string
	originalQuery   string
	originalHost    string
	originalHeader  http.Header
	body            []byte
	claims          *middleware.TokenClaims
}

// Director configures the reverse proxy request for a selected route.
type Director struct {
	cacheManager *cache.Manager
	attrCache    *middleware.AttributionCache
	router       *ModelRouter
}

func NewDirector(cacheManager *cache.Manager, attrCache *middleware.AttributionCache, router *ModelRouter) *Director {
	return &Director{cacheManager: cacheManager, attrCache: attrCache, router: router}
}

func (d *Director) Direct(req *http.Request) {
	claims, ok := middleware.ClaimsFromContext(req.Context())
	if !ok {
		logging.Capture(req.Context(), fmt.Errorf("proxy director: missing claims on %s", req.URL.Path))
		req.Header.Set("X-Proxy-Error", "missing claims")
		return
	}

	if d.router != nil && claims.IsSystem {
		if plan, ok := d.newRoutePlan(req, claims); ok {
			*req = *req.WithContext(context.WithValue(req.Context(), routePlanContextKey{}, plan))
			if err := d.applyCandidate(req, plan, 0); err != nil {
				directorError(req, err)
			}
			return
		}
		if req.Header.Get("X-Proxy-Error") != "" {
			return
		}
	}

	directLegacy(req, d.cacheManager, d.attrCache, claims)
}

func (d *Director) newRoutePlan(req *http.Request, claims *middleware.TokenClaims) (*routePlan, bool) {
	body, err := snapshotRequestBody(req)
	if err != nil {
		directorError(req, fmt.Errorf("read request body: %w", err))
		return nil, false
	}
	payload, canonicalModel, ok := parseModelJSONBody(body)
	if !ok || canonicalModel == "" {
		return nil, false
	}
	var streaming bool
	if raw, exists := payload["stream"]; exists {
		_ = json.Unmarshal(raw, &streaming)
	}
	candidates, err := d.router.Candidates(req.Context(), claims, canonicalModel)
	if err != nil {
		directorError(req, err)
		return nil, false
	}
	if len(candidates) == 0 {
		return nil, false
	}
	return &routePlan{
		canonicalModel:  canonicalModel,
		candidates:      candidates,
		streaming:       streaming,
		originalPath:    req.URL.Path,
		originalRawPath: req.URL.RawPath,
		originalQuery:   req.URL.RawQuery,
		originalHost:    req.Host,
		originalHeader:  req.Header.Clone(),
		body:            body,
		claims:          claims,
	}, true
}

func snapshotRequestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	resetRequestBody(req, body)
	return body, nil
}

func resetRequestBody(req *http.Request, body []byte) {
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
}

func (d *Director) applyCandidate(req *http.Request, plan *routePlan, index int) error {
	if index < 0 || index >= len(plan.candidates) {
		return ErrNoHealthyRoute
	}
	plan.index = index
	candidate := plan.candidates[index]
	req.URL.Path = plan.originalPath
	req.URL.RawPath = plan.originalRawPath
	req.URL.RawQuery = plan.originalQuery
	req.Host = plan.originalHost
	req.Header = plan.originalHeader.Clone()
	resetRequestBody(req, plan.body)

	cred, err := d.cacheManager.GetDecryptedCredentialByID(req.Context(), candidate.CredentialID)
	if err != nil {
		return fmt.Errorf("load route credential %s: %w", candidate.CredentialID, err)
	}
	if cred.ProviderID != candidate.ProviderID {
		return fmt.Errorf("route credential provider mismatch")
	}
	if err := configureRequest(req, cred, d.attrCache, plan.claims); err != nil {
		return err
	}
	if err := RewriteModel(req, candidate.UpstreamID); err != nil {
		return fmt.Errorf("rewrite model for provider %q: %w", candidate.ProviderID, err)
	}
	if captured, ok := observe.CapturedDataFromContext(req.Context()); ok {
		candidateModelID := candidate.CanonicalModelID
		if candidateModelID == "" {
			candidateModelID = plan.canonicalModel
		}
		captured.Usage = observe.UsageData{}
		captured.Model = candidateModelID
		captured.ProviderID = candidate.ProviderID
		captured.CredentialID = candidate.CredentialID
		captured.GenerationID = ""
		captured.IsStreaming = false
		captured.TTFBMs = 0
		captured.TotalMs = 0
		captured.ErrorType = ""
		captured.ErrorMessage = ""
		captured.UpstreamStatus = 0
	}
	return nil
}

func directLegacy(req *http.Request, cacheManager *cache.Manager, attrCache *middleware.AttributionCache, claims *middleware.TokenClaims) {
	cred, err := cacheManager.GetDecryptedCredentialByID(req.Context(), claims.CredentialID)
	if err != nil {
		directorError(req, fmt.Errorf("credential lookup %s: %w", claims.CredentialID, err))
		return
	}
	if err := configureRequest(req, cred, attrCache, claims); err != nil {
		directorError(req, err)
		return
	}
	modelName, _, err := RewriteRoutedModel(req, cred.ProviderID)
	if err != nil {
		directorError(req, fmt.Errorf("rewrite model for provider %q: %w", cred.ProviderID, err))
		return
	}
	if captured, ok := observe.CapturedDataFromContext(req.Context()); ok {
		captured.Model = modelName
		captured.ProviderID = cred.ProviderID
		captured.CredentialID = claims.CredentialID
	}
}

func configureRequest(req *http.Request, cred *cache.DecryptedCredential, attrCache *middleware.AttributionCache, claims *middleware.TokenClaims) error {
	if err := ValidateBaseURL(cred.BaseURL); err != nil {
		return fmt.Errorf("disallowed upstream base URL: %w", err)
	}
	for _, h := range []string{
		"Metadata-Flavor", "X-Aws-Ec2-Metadata-Token", "X-Aws-Ec2-Metadata-Token-Ttl-Seconds", "Metadata",
		"CF-Connecting-IP", "CF-Connecting-IPv6", "CF-Ray", "CF-Visitor", "CF-IPCountry", "CF-Worker",
		"CDN-Loop", "True-Client-IP", "X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host", "X-Real-IP",
	} {
		req.Header.Del(h)
	}

	upstreamPath := stripProxyPrefix(req.URL.Path)
	baseURL := strings.TrimRight(cred.BaseURL, "/")
	req.URL.Scheme = "https"
	if strings.HasPrefix(baseURL, "http://") {
		req.URL.Scheme = "http"
		baseURL = strings.TrimPrefix(baseURL, "http://")
	} else {
		baseURL = strings.TrimPrefix(baseURL, "https://")
	}
	hostAndPath := strings.SplitN(baseURL, "/", 2)
	req.URL.Host = hostAndPath[0]
	basePath := ""
	if len(hostAndPath) > 1 {
		basePath = "/" + hostAndPath[1]
	}
	req.URL.Path = joinUpstreamPath(basePath, upstreamPath)
	req.Host = hostAndPath[0]

	req.Header.Del("Authorization")
	AttachAuth(req, cred.AuthScheme, cred.APIKey)
	if err := applyUsageAccounting(req, cred.ProviderID, cred.BaseURL, openRouterEndUser(attrCache, claims.JTI)); err != nil {
		logging.Capture(req.Context(), fmt.Errorf("proxy director: force usage accounting: %w", err))
	}
	for i := range cred.APIKey {
		cred.APIKey[i] = 0
	}
	req.Header.Set("X-Request-ID", uuid.New().String())
	return nil
}

func applyUsageAccounting(req *http.Request, providerID, baseURL, endUserID string) error {
	if providerheaders.IsOpenRouter(providerID, baseURL) {
		providerheaders.ApplyOpenRouter(req)
		return EnsureOpenRouterUsage(req, endUserID)
	}
	if providerID == "xiaomi" || providerID == "atlascloud" || providerID == "novita" || providerID == "engy" {
		return EnsureOpenAICompatibleUsage(req)
	}
	return nil
}

func directorError(req *http.Request, err error) {
	logging.Capture(req.Context(), fmt.Errorf("proxy director: %w", err))
	req.Header.Set("X-Proxy-Error", "proxy configuration failed")
}

func routePlanFromContext(ctx context.Context) (*routePlan, bool) {
	plan, ok := ctx.Value(routePlanContextKey{}).(*routePlan)
	return plan, ok
}

func openRouterEndUser(attrCache *middleware.AttributionCache, jti string) string {
	if attrCache == nil {
		return ""
	}
	attr, ok := attrCache.Get(jti)
	if !ok || attr.SessionID == nil {
		return ""
	}
	return attr.SessionID.String()
}

// stripProxyPrefix removes the /v1/proxy prefix from the path.
func stripProxyPrefix(path string) string {
	after := strings.TrimPrefix(path, "/v1/proxy")
	if after == "" {
		return "/"
	}
	return after
}

func joinUpstreamPath(basePath, upstreamPath string) string {
	for _, prefix := range []string{"/api/v1", "/api/v2", "/v1", "/v2"} {
		if strings.HasSuffix(basePath, prefix) && strings.HasPrefix(upstreamPath, prefix+"/") {
			return basePath + strings.TrimPrefix(upstreamPath, prefix)
		}
		if strings.HasSuffix(basePath, prefix) && upstreamPath == prefix {
			return basePath
		}
	}
	return basePath + upstreamPath
}
