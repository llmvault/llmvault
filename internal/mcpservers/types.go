package mcpservers

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// OAuthMetadata contains only public OAuth discovery and client configuration.
// Secrets and tokens belong in AuthorizationInput and are encrypted at rest.
type OAuthMetadata struct {
	Resource                          string   `json:"resource,omitempty"`
	ProtectedResourceURL              string   `json:"protected_resource_metadata_url,omitempty"`
	Issuer                            string   `json:"issuer,omitempty"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint,omitempty"`
	TokenEndpoint                     string   `json:"token_endpoint,omitempty"`
	RegistrationEndpoint              string   `json:"registration_endpoint,omitempty"`
	ScopesSupported                   []string `json:"scopes_supported,omitempty"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported,omitempty"`
	ClientIDMetadataDocumentSupported bool     `json:"client_id_metadata_document_supported,omitempty"`
}

type CreateServerParams struct {
	Scope               string
	Name                string
	Slug                string
	Description         string
	URL                 string
	Transport           string
	AuthType            string
	AuthorizationPolicy string
	HeaderName          string
	OAuthMetadata       OAuthMetadata
	Authorization       *AuthorizationInput
}

type UpdateServerParams struct {
	Name                *string
	Slug                *string
	Description         *string
	URL                 *string
	Transport           *string
	AuthType            *string
	AuthorizationPolicy *string
	HeaderName          *string
	OAuthMetadata       *OAuthMetadata
	Status              *string
}

type ListServersInput struct {
	Limit           int
	IncludeOrg      bool
	BeforeCreatedAt *time.Time
	BeforeID        *uuid.UUID
}

type ListServersResult struct {
	Servers []model.MCPServer
	HasMore bool
}

// AuthorizationInput accepts all supported credential shapes. Only fields for
// the server's AuthType are retained in the encrypted credential envelope.
type AuthorizationInput struct {
	PrincipalType    string
	BearerToken      string
	HeaderValue      string
	AccessToken      string
	RefreshToken     string
	ClientID         string
	ClientSecret     string
	Scopes           []string
	TokenType        string
	ExpiresAt        *time.Time
	RefreshExpiresAt *time.Time
	Status           string
}

type AuthorizationSummary struct {
	ID               uuid.UUID  `json:"id"`
	PrincipalType    string     `json:"principal_type"`
	PrincipalUserID  *uuid.UUID `json:"principal_user_id,omitempty"`
	AuthType         string     `json:"auth_type"`
	Configured       bool       `json:"configured"`
	ClientID         string     `json:"client_id,omitempty"`
	Scopes           []string   `json:"scopes"`
	TokenType        string     `json:"token_type,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	RefreshExpiresAt *time.Time `json:"refresh_expires_at,omitempty"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type AuthorizationSummaries struct {
	User    *AuthorizationSummary
	Service *AuthorizationSummary
}

type ServerView struct {
	Server               model.MCPServer
	UserAuthorization    *AuthorizationSummary
	ServiceAuthorization *AuthorizationSummary
}

type AgentGrant struct {
	MCPServerID uuid.UUID `json:"mcp_server_id"`
	State       string    `json:"state"`
}

// RuntimeServer is the only decrypted service output. The caller must keep
// Headers server-side until it serializes the short-lived runtime definition;
// no REST response type embeds this structure.
type RuntimeServer struct {
	ID        uuid.UUID
	Name      string
	Scope     string
	URL       string
	Transport string
	Headers   map[string]string
}

type OAuthStartParams struct {
	PrincipalType string
	ClientID      string
	ClientSecret  string
	Scopes        []string
	RedirectAfter string
}

type OAuthStartResult struct {
	AuthorizationURL string
	ExpiresAt        time.Time
}

type OAuthCallbackResult struct {
	OrgID         uuid.UUID
	MCPServerID   uuid.UUID
	UserID        uuid.UUID
	PrincipalType string
	RedirectAfter string
}

type OAuthClientMetadata struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

type credentialEnvelope struct {
	BearerToken  string `json:"bearer_token,omitempty"`
	HeaderValue  string `json:"header_value,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
}

func encodeOAuthMetadata(metadata OAuthMetadata) model.JSON {
	raw, _ := json.Marshal(metadata)
	var result model.JSON
	_ = json.Unmarshal(raw, &result)
	return result
}

func decodeOAuthMetadata(raw model.JSON) OAuthMetadata {
	data, _ := json.Marshal(raw)
	var result OAuthMetadata
	_ = json.Unmarshal(data, &result)
	return result
}

// DecodeOAuthMetadataForAPI exposes the sanitized public metadata shape to the
// handler package without exposing the encrypted authorization internals.
func DecodeOAuthMetadataForAPI(raw model.JSON) OAuthMetadata { return decodeOAuthMetadata(raw) }
