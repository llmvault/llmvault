package handler

import (
	"time"

	"github.com/usehivy/hivy/internal/mcpservers"
	"github.com/usehivy/hivy/internal/model"
)

type mcpServerResponse struct {
	ID                   string                           `json:"id"`
	Scope                string                           `json:"scope"`
	OwnerUserID          *string                          `json:"owner_user_id,omitempty"`
	Name                 string                           `json:"name"`
	Slug                 string                           `json:"slug"`
	Description          string                           `json:"description"`
	URL                  string                           `json:"url"`
	Transport            string                           `json:"transport"`
	AuthType             string                           `json:"auth_type"`
	AuthorizationPolicy  string                           `json:"authorization_policy"`
	HeaderName           string                           `json:"header_name,omitempty"`
	OAuthMetadata        mcpservers.OAuthMetadata         `json:"oauth_metadata"`
	Status               string                           `json:"status"`
	UserAuthorization    *mcpservers.AuthorizationSummary `json:"user_authorization,omitempty"`
	ServiceAuthorization *mcpservers.AuthorizationSummary `json:"service_authorization,omitempty"`
	CreatedAt            time.Time                        `json:"created_at"`
	UpdatedAt            time.Time                        `json:"updated_at"`
}

type mcpServersResponse struct {
	MCPServers []mcpServerResponse `json:"mcp_servers"`
	NextCursor *string             `json:"next_cursor,omitempty"`
	HasMore    bool                `json:"has_more"`
}

type mcpServerEnvelope struct {
	MCPServer mcpServerResponse `json:"mcp_server"`
}

type mcpAuthorizationRequest struct {
	PrincipalType    string     `json:"principal_type"`
	BearerToken      string     `json:"bearer_token,omitempty"`
	HeaderValue      string     `json:"header_value,omitempty"`
	AccessToken      string     `json:"access_token,omitempty"`
	RefreshToken     string     `json:"refresh_token,omitempty"`
	ClientID         string     `json:"client_id,omitempty"`
	ClientSecret     string     `json:"client_secret,omitempty"`
	Scopes           []string   `json:"scopes,omitempty"`
	TokenType        string     `json:"token_type,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	RefreshExpiresAt *time.Time `json:"refresh_expires_at,omitempty"`
	Status           string     `json:"status,omitempty"`
}

type createMCPServerRequest struct {
	Scope               string                   `json:"scope"`
	Name                string                   `json:"name"`
	Slug                string                   `json:"slug,omitempty"`
	Description         string                   `json:"description,omitempty"`
	URL                 string                   `json:"url"`
	Transport           string                   `json:"transport,omitempty"`
	AuthType            string                   `json:"auth_type,omitempty"`
	AuthorizationPolicy string                   `json:"authorization_policy,omitempty"`
	HeaderName          string                   `json:"header_name,omitempty"`
	OAuthMetadata       mcpservers.OAuthMetadata `json:"oauth_metadata,omitempty"`
	Authorization       *mcpAuthorizationRequest `json:"authorization,omitempty"`
}

type updateMCPServerRequest struct {
	Name                *string                   `json:"name,omitempty"`
	Slug                *string                   `json:"slug,omitempty"`
	Description         *string                   `json:"description,omitempty"`
	URL                 *string                   `json:"url,omitempty"`
	Transport           *string                   `json:"transport,omitempty"`
	AuthType            *string                   `json:"auth_type,omitempty"`
	AuthorizationPolicy *string                   `json:"authorization_policy,omitempty"`
	HeaderName          *string                   `json:"header_name,omitempty"`
	OAuthMetadata       *mcpservers.OAuthMetadata `json:"oauth_metadata,omitempty"`
	Status              *string                   `json:"status,omitempty"`
}

type mcpAuthorizationEnvelope struct {
	Authorization mcpservers.AuthorizationSummary `json:"authorization"`
}

type mcpOAuthStartRequest struct {
	PrincipalType string   `json:"principal_type,omitempty"`
	ClientID      string   `json:"client_id,omitempty"`
	ClientSecret  string   `json:"client_secret,omitempty"`
	Scopes        []string `json:"scopes,omitempty"`
	RedirectAfter string   `json:"redirect_after,omitempty"`
}

type mcpOAuthStartResponse struct {
	AuthorizationURL string    `json:"authorization_url"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type mcpOAuthCallbackResponse struct {
	Status      string `json:"status"`
	MCPServerID string `json:"mcp_server_id"`
}

type mcpServerIDRequest struct {
	MCPServerID string `json:"mcp_server_id"`
}

type mcpAgentGrantRequest struct {
	MCPServerID string `json:"mcp_server_id"`
	State       string `json:"state"`
}

type mcpAgentGrantResponse struct {
	MCPServerID string `json:"mcp_server_id"`
	State       string `json:"state"`
}

type mcpAgentGrantsResponse struct {
	MCPServers []mcpAgentGrantResponse `json:"mcp_servers"`
}

func mcpAuthorizationInput(request mcpAuthorizationRequest) mcpservers.AuthorizationInput {
	return mcpservers.AuthorizationInput{
		PrincipalType: request.PrincipalType, BearerToken: request.BearerToken,
		HeaderValue: request.HeaderValue, AccessToken: request.AccessToken,
		RefreshToken: request.RefreshToken, ClientID: request.ClientID,
		ClientSecret: request.ClientSecret, Scopes: request.Scopes,
		TokenType: request.TokenType, ExpiresAt: request.ExpiresAt,
		RefreshExpiresAt: request.RefreshExpiresAt, Status: request.Status,
	}
}

func mcpServerView(server model.MCPServer, user, service *mcpservers.AuthorizationSummary) mcpServerResponse {
	var owner *string
	if server.OwnerUserID != nil {
		value := server.OwnerUserID.String()
		owner = &value
	}
	return mcpServerResponse{
		ID: server.ID.String(), Scope: server.Scope, OwnerUserID: owner,
		Name: server.Name, Slug: server.Slug, Description: server.Description,
		URL: server.URL, Transport: server.Transport, AuthType: server.AuthType,
		AuthorizationPolicy: server.AuthorizationPolicy, HeaderName: server.HeaderName,
		OAuthMetadata: mcpservers.DecodeOAuthMetadataForAPI(server.OAuthMetadata), Status: server.Status,
		UserAuthorization: user, ServiceAuthorization: service,
		CreatedAt: server.CreatedAt, UpdatedAt: server.UpdatedAt,
	}
}
