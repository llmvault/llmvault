package nango

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/usehivy/hivy/internal/logging"
)

// Client wraps the Nango API for discovering configured integrations and
// managing end-user connections.
// Authenticated via a secret key (UUID v4) with Bearer token auth.
type Client struct {
	endpoint   string
	secretKey  string
	httpClient *http.Client
	mu         sync.RWMutex
	providers  map[string]Provider       // cached provider catalog
	templates  map[string]map[string]any // raw provider templates for config extraction
}

// Provider represents a Nango integration provider from the catalog.
type Provider struct {
	Name                     string `json:"name"`
	DisplayName              string `json:"display_name"`
	AuthMode                 string `json:"auth_mode"`
	ClientRegistration       string `json:"client_registration,omitempty"` // for MCP_OAUTH2
	WebhookUserDefinedSecret bool   `json:"webhook_user_defined_secret,omitempty"`
}

// Integration is a configured provider integration returned by Nango.
// UniqueKey is Nango's provider configuration key and Provider identifies the
// underlying provider template (for example github-app).
type Integration struct {
	UniqueKey   string `json:"unique_key"`
	Provider    string `json:"provider"`
	DisplayName string `json:"display_name"`
	Logo        string `json:"logo,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// NewClient creates a Nango API client.
func NewClient(endpoint, secretKey string) *Client {
	return &Client{
		endpoint:   endpoint,
		secretKey:  secretKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		providers:  make(map[string]Provider),
		templates:  make(map[string]map[string]any),
	}
}

// FetchProviders fetches the full provider catalog from Nango and caches it.
// Called on startup and can be called periodically to refresh.
func (c *Client) FetchProviders(ctx context.Context) error {
	resp, err := c.doJSON(ctx, http.MethodGet, "/providers", nil)
	if err != nil {
		return fmt.Errorf("fetching providers: %w", err)
	}

	rawData, ok := resp["data"]
	if !ok {
		return fmt.Errorf("unexpected provider response: missing 'data' key")
	}

	b, err := json.Marshal(rawData)
	if err != nil {
		return fmt.Errorf("marshaling provider data: %w", err)
	}

	var providers []Provider
	if err := json.Unmarshal(b, &providers); err != nil {
		return fmt.Errorf("unmarshaling providers: %w", err)
	}

	var rawProviders []map[string]any
	if err := json.Unmarshal(b, &rawProviders); err != nil {
		return fmt.Errorf("unmarshaling raw provider templates: %w", err)
	}
	templates := make(map[string]map[string]any, len(rawProviders))
	for _, rp := range rawProviders {
		if name, ok := rp["name"].(string); ok {
			templates[name] = rp
		}
	}

	catalog := make(map[string]Provider, len(providers))
	for _, p := range providers {
		if tmpl, ok := templates[p.Name]; ok {
			if wuds, ok := tmpl["webhook_user_defined_secret"].(bool); ok {
				p.WebhookUserDefinedSecret = wuds
			}
		}
		catalog[p.Name] = p
	}

	c.mu.Lock()
	c.providers = catalog
	c.templates = templates
	c.mu.Unlock()

	logging.FromContext(ctx).InfoContext(ctx, "nango client initialized", "providers", len(catalog))
	return nil
}

// GetProvider returns a cached provider by name. Thread-safe.
func (c *Client) GetProvider(name string) (Provider, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.providers[name]
	return p, ok
}

// GetProviderTemplate returns the raw provider template for config extraction.
func (c *Client) GetProviderTemplate(name string) (map[string]any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	t, ok := c.templates[name]
	return t, ok
}

// CallbackURL returns the Nango OAuth callback URL.
func (c *Client) CallbackURL() string {
	return c.endpoint + "/oauth/callback"
}

// GetProviders returns all cached providers as a slice.
func (c *Client) GetProviders() []Provider {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]Provider, 0, len(c.providers))
	for _, p := range c.providers {
		result = append(result, p)
	}
	return result
}

// ListIntegrations returns every integration configured in Nango.
// GET /integrations
func (c *Client) ListIntegrations(ctx context.Context) ([]Integration, error) {
	resp, err := c.doJSON(ctx, http.MethodGet, "/integrations", nil)
	if err != nil {
		return nil, fmt.Errorf("list Nango integrations: %w", err)
	}
	raw, ok := resp["data"]
	if !ok {
		return nil, fmt.Errorf("unexpected integration response: missing 'data' key")
	}
	body, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal Nango integrations: %w", err)
	}
	var integrations []Integration
	if err := json.Unmarshal(body, &integrations); err != nil {
		return nil, fmt.Errorf("unmarshal Nango integrations: %w", err)
	}
	for _, integration := range integrations {
		if integration.UniqueKey == "" || integration.Provider == "" || integration.DisplayName == "" {
			return nil, fmt.Errorf("unexpected integration response: unique_key, provider, and display_name are required")
		}
	}
	sort.Slice(integrations, func(i, j int) bool { return integrations[i].UniqueKey < integrations[j].UniqueKey })
	return integrations, nil
}

// GetIntegration fetches an integration by its unique key.
// GET /integrations/{uniqueKey}?include[]=webhook&include[]=credentials
func (c *Client) GetIntegration(ctx context.Context, uniqueKey string) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodGet, "/integrations/"+uniqueKey+"?include[]=webhook&include[]=credentials", nil)
}

// DeleteConnection removes a connection by its ID.
// DELETE /connection/{connectionId}?provider_config_key={providerConfigKey}
func (c *Client) DeleteConnection(ctx context.Context, connectionID, providerConfigKey string) error {
	path := fmt.Sprintf("/connection/%s?provider_config_key=%s", connectionID, providerConfigKey)
	if _, err := c.doJSON(ctx, http.MethodDelete, path, nil); err != nil {
		logging.Capture(ctx, fmt.Errorf("nango delete connection %s: %w", connectionID, err))
		return err
	}
	return nil
}

// CreateConnectionRequest is the payload for creating a connection directly in Nango.
type CreateConnectionRequest struct {
	ProviderConfigKey string `json:"provider_config_key"`
	ConnectionID      string `json:"connection_id"`
	APIKey            string `json:"api_key,omitempty"`
}

// CreateConnection creates a connection directly in Nango (e.g. for API_KEY auth mode).
// POST /connection
func (c *Client) CreateConnection(ctx context.Context, req CreateConnectionRequest) error {
	if _, err := c.doJSON(ctx, http.MethodPost, "/connection", req); err != nil {
		logging.Capture(ctx, fmt.Errorf("nango create connection %s: %w", req.ConnectionID, err))
		return err
	}
	return nil
}

// GetConnection retrieves a connection from Nango.
// GET /connection/{connectionId}?provider_config_key={providerConfigKey}
func (c *Client) GetConnection(ctx context.Context, connectionID, providerConfigKey string) (map[string]any, error) {
	path := fmt.Sprintf("/connection/%s?provider_config_key=%s", connectionID, providerConfigKey)
	return c.doJSON(ctx, http.MethodGet, path, nil)
}
