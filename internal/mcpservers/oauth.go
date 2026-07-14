package mcpservers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type protectedResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
	ScopesSupported      []string `json:"scopes_supported"`
}

type authorizationServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	ScopesSupported                   []string `json:"scopes_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	ClientIDMetadataDocumentSupported bool     `json:"client_id_metadata_document_supported"`
}

type dynamicClientRegistrationResponse struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type oauthTokenResponse struct {
	AccessToken  string   `json:"access_token"`
	TokenType    string   `json:"token_type"`
	ExpiresIn    int64    `json:"expires_in"`
	RefreshToken string   `json:"refresh_token"`
	Scope        string   `json:"scope"`
	Scopes       []string `json:"scopes"`
}

func (s *Service) StartOAuth(ctx context.Context, orgID, serverID, actorUserID uuid.UUID, params OAuthStartParams) (*OAuthStartResult, error) {
	if s.callbackURL == "" {
		return nil, validationErrorf("MCP OAuth callback URL is not configured")
	}
	server, err := s.GetServer(ctx, orgID, serverID, &actorUserID)
	if err != nil {
		return nil, err
	}
	if server.AuthType != model.MCPAuthTypeOAuthAuthorizationCode {
		return nil, validationErrorf("server does not use OAuth authorization code authentication")
	}
	principal, _, err := normalizePrincipal(*server, actorUserID, params.PrincipalType)
	if err != nil {
		return nil, err
	}
	input := AuthorizationInput{
		PrincipalType: principal,
		ClientID:      strings.TrimSpace(params.ClientID),
		ClientSecret:  params.ClientSecret,
		Scopes:        sortedUnique(params.Scopes),
		Status:        model.MCPAuthorizationStatusActive,
	}
	if existing, loadErr := s.getAuthorization(ctx, orgID, serverID, principal, actorUserID); loadErr == nil {
		envelope, decryptErr := s.decryptEnvelope(existing.CredentialsEncrypted)
		if decryptErr != nil {
			return nil, decryptErr
		}
		if input.ClientID == "" {
			input.ClientID = existing.ClientID
		}
		if input.ClientSecret == "" {
			input.ClientSecret = envelope.ClientSecret
		}
		if len(input.Scopes) == 0 {
			input.Scopes = append([]string{}, existing.Scopes...)
		}
		input.AccessToken = envelope.AccessToken
		input.RefreshToken = envelope.RefreshToken
		input.TokenType = existing.TokenType
		input.ExpiresAt = existing.ExpiresAt
		input.RefreshExpiresAt = existing.RefreshExpiresAt
	} else if !errors.Is(loadErr, ErrAuthorizationNotFound) {
		return nil, loadErr
	}
	// An org OAuth server may publish one manager-configured OAuth client while
	// requiring each member to authorize their own external identity. In that
	// shape a new user inherits only registration fields from the org-service
	// record. Upstream access and refresh tokens are deliberately never copied.
	if principal == model.MCPPrincipalUser && server.Scope == model.MCPServerScopeOrg && input.ClientID == "" {
		registration, loadErr := s.getAuthorization(ctx, orgID, serverID, model.MCPPrincipalOrgService, uuid.Nil)
		if loadErr == nil {
			registrationSecret, decryptErr := s.decryptEnvelope(registration.CredentialsEncrypted)
			if decryptErr != nil {
				return nil, decryptErr
			}
			input.ClientID = registration.ClientID
			input.ClientSecret = registrationSecret.ClientSecret
			if len(input.Scopes) == 0 {
				input.Scopes = append([]string{}, registration.Scopes...)
			}
		} else if !errors.Is(loadErr, ErrAuthorizationNotFound) {
			return nil, loadErr
		}
	}
	metadata, err := s.DiscoverOAuth(ctx, *server)
	if err != nil {
		return nil, err
	}
	if metadata.AuthorizationEndpoint == "" || metadata.TokenEndpoint == "" {
		return nil, validationErrorf("OAuth server metadata is missing authorization or token endpoint")
	}
	if input.ClientID == "" && metadata.ClientIDMetadataDocumentSupported {
		input.ClientID = s.ClientMetadataURL()
	}
	if input.ClientID == "" && metadata.RegistrationEndpoint != "" {
		registration, err := s.registerOAuthClient(ctx, metadata.RegistrationEndpoint, input.Scopes)
		if err != nil {
			return nil, err
		}
		input.ClientID = registration.ClientID
		input.ClientSecret = registration.ClientSecret
	}
	if input.ClientID == "" {
		return nil, validationErrorf("OAuth server requires a pre-registered client_id")
	}
	if err := s.upsertAuthorization(ctx, s.db, *server, actorUserID, input); err != nil {
		return nil, err
	}
	state, err := randomURLToken(32)
	if err != nil {
		return nil, fmt.Errorf("generate oauth state: %w", err)
	}
	verifier, err := randomURLToken(64)
	if err != nil {
		return nil, fmt.Errorf("generate PKCE verifier: %w", err)
	}
	encryptedVerifier, err := s.encKey.EncryptString(verifier)
	if err != nil {
		return nil, fmt.Errorf("encrypt PKCE verifier: %w", err)
	}
	expiresAt := s.now().Add(10 * time.Minute)
	redirectAfter := safeRedirectAfter(params.RedirectAfter)
	row := model.MCPOAuthState{
		OrgID: orgID, MCPServerID: serverID, UserID: actorUserID, PrincipalType: principal,
		StateHash: hashState(state), EncryptedVerifier: encryptedVerifier,
		RedirectAfter: redirectAfter, ExpiresAt: expiresAt,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, fmt.Errorf("store oauth state: %w", err)
	}
	challenge := sha256.Sum256([]byte(verifier))
	values := url.Values{
		"response_type":         {"code"},
		"client_id":             {input.ClientID},
		"redirect_uri":          {s.callbackURL},
		"state":                 {state},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
	}
	resource := metadata.Resource
	if resource == "" {
		resource = server.URL
	}
	values.Set("resource", resource)
	if len(input.Scopes) > 0 {
		values.Set("scope", strings.Join(input.Scopes, " "))
	}
	validatedAuthorizationEndpoint, err := normalizeEndpointURL(metadata.AuthorizationEndpoint)
	if err != nil {
		return nil, err
	}
	authorizationURL, err := url.Parse(validatedAuthorizationEndpoint)
	if err != nil {
		return nil, validationErrorf("authorization endpoint is invalid")
	}
	query := authorizationURL.Query()
	for key, list := range values {
		for _, value := range list {
			query.Add(key, value)
		}
	}
	authorizationURL.RawQuery = query.Encode()
	return &OAuthStartResult{AuthorizationURL: authorizationURL.String(), ExpiresAt: expiresAt}, nil
}

// CompleteOAuth consumes a single-use state and exchanges code using PKCE.
// The callback is intentionally state-authenticated and does not require org
// middleware, because OAuth providers cannot supply Hivy's active-org header.
