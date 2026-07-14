package mcpservers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

func (s *Service) CompleteOAuth(ctx context.Context, rawState, code string) (*OAuthCallbackResult, error) {
	if strings.TrimSpace(rawState) == "" || strings.TrimSpace(code) == "" {
		return nil, ErrOAuthStateInvalid
	}
	var state model.MCPOAuthState
	var server model.MCPServer
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("state_hash = ? AND used_at IS NULL AND expires_at > ?", hashState(rawState), s.now()).
			First(&state).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrOAuthStateInvalid
		} else if err != nil {
			return fmt.Errorf("load oauth state: %w", err)
		}
		if err := tx.WithContext(ctx).Where("id = ? AND org_id = ?", state.MCPServerID, state.OrgID).First(&server).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrOAuthStateInvalid
		} else if err != nil {
			return fmt.Errorf("load oauth state server: %w", err)
		}
		result := tx.WithContext(ctx).Model(&model.MCPOAuthState{}).
			Where("id = ? AND used_at IS NULL", state.ID).Update("used_at", s.now())
		if result.Error != nil {
			return fmt.Errorf("consume oauth state: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrOAuthStateInvalid
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	verifier, err := s.encKey.DecryptString(state.EncryptedVerifier)
	if err != nil {
		return nil, fmt.Errorf("decrypt PKCE verifier: %w", err)
	}
	authorization, err := s.getAuthorization(ctx, state.OrgID, state.MCPServerID, state.PrincipalType, state.UserID)
	if err != nil {
		return nil, err
	}
	envelope, err := s.decryptEnvelope(authorization.CredentialsEncrypted)
	if err != nil {
		return nil, err
	}
	metadata, err := s.DiscoverOAuth(ctx, server)
	if err != nil {
		return nil, err
	}
	values := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {s.callbackURL},
		"code_verifier": {verifier},
		"client_id":     {authorization.ClientID},
	}
	resource := metadata.Resource
	if resource == "" {
		resource = server.URL
	}
	values.Set("resource", resource)
	token, err := s.requestToken(ctx, metadata, authorization.ClientID, envelope.ClientSecret, values)
	if err != nil {
		return nil, err
	}
	if err := s.saveToken(ctx, authorization, envelope, token); err != nil {
		return nil, err
	}
	return &OAuthCallbackResult{
		OrgID: state.OrgID, MCPServerID: state.MCPServerID, UserID: state.UserID,
		PrincipalType: state.PrincipalType, RedirectAfter: state.RedirectAfter,
	}, nil
}

func (s *Service) DiscoverOAuth(ctx context.Context, server model.MCPServer) (OAuthMetadata, error) {
	metadata := decodeOAuthMetadata(server.OAuthMetadata)
	if metadata.Resource == "" {
		metadata.Resource = server.URL
	}
	metadataComplete := metadata.TokenEndpoint != "" &&
		(server.AuthType == model.MCPAuthTypeOAuthClientCredentials || metadata.AuthorizationEndpoint != "")
	if metadataComplete {
		if err := validateOAuthMetadataEndpoints(metadata); err != nil {
			return OAuthMetadata{}, err
		}
		return metadata, nil
	}
	resourceURL := metadata.ProtectedResourceURL
	if resourceURL == "" {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		if err != nil {
			return OAuthMetadata{}, fmt.Errorf("create MCP OAuth challenge request: %w", err)
		}
		request.Header.Set("Accept", "application/json")
		response, err := s.httpClient.Do(request)
		if err == nil {
			resourceURL = bearerParameter(response.Header.Get("WWW-Authenticate"), "resource_metadata")
			_, _ = io.CopyN(io.Discard, response.Body, 4096)
			_ = response.Body.Close()
		}
	}
	var protected protectedResourceMetadata
	if resourceURL != "" {
		if err := s.fetchJSON(ctx, resourceURL, &protected); err != nil {
			return OAuthMetadata{}, fmt.Errorf("discover MCP protected resource metadata: %w", err)
		}
	} else {
		var discoveryErr error
		for _, candidate := range protectedResourceMetadataURLs(server.URL) {
			if err := s.fetchJSON(ctx, candidate, &protected); err == nil {
				resourceURL = candidate
				break
			} else {
				discoveryErr = err
			}
		}
		if resourceURL == "" {
			return OAuthMetadata{}, fmt.Errorf("discover MCP protected resource metadata: %w", discoveryErr)
		}
	}
	if protected.Resource != "" {
		metadata.Resource = protected.Resource
	}
	metadata.ProtectedResourceURL = resourceURL
	if len(metadata.ScopesSupported) == 0 {
		metadata.ScopesSupported = protected.ScopesSupported
	}
	issuer := metadata.Issuer
	if issuer == "" && len(protected.AuthorizationServers) > 0 {
		issuer = protected.AuthorizationServers[0]
	}
	if issuer == "" {
		return OAuthMetadata{}, validationErrorf("protected resource metadata has no authorization server")
	}
	metadata.Issuer = issuer
	wellKnown, err := authorizationMetadataURL(issuer, "/.well-known/oauth-authorization-server")
	if err != nil {
		return OAuthMetadata{}, err
	}
	var authorization authorizationServerMetadata
	if err := s.fetchJSON(ctx, wellKnown, &authorization); err != nil {
		openidURL, urlErr := authorizationMetadataURL(issuer, "/.well-known/openid-configuration")
		if urlErr != nil {
			return OAuthMetadata{}, urlErr
		}
		if oidcErr := s.fetchJSON(ctx, openidURL, &authorization); oidcErr != nil {
			return OAuthMetadata{}, fmt.Errorf("discover OAuth authorization server metadata: %w", err)
		}
	}
	metadata.AuthorizationEndpoint = authorization.AuthorizationEndpoint
	metadata.TokenEndpoint = authorization.TokenEndpoint
	metadata.RegistrationEndpoint = authorization.RegistrationEndpoint
	metadata.TokenEndpointAuthMethodsSupported = sortedUnique(authorization.TokenEndpointAuthMethodsSupported)
	metadata.ClientIDMetadataDocumentSupported = authorization.ClientIDMetadataDocumentSupported
	if authorization.Issuer != "" {
		if !sameOAuthIssuer(issuer, authorization.Issuer) {
			return OAuthMetadata{}, validationErrorf("OAuth authorization server issuer does not match discovery")
		}
		metadata.Issuer = authorization.Issuer
	}
	if len(metadata.ScopesSupported) == 0 {
		metadata.ScopesSupported = authorization.ScopesSupported
	}
	if metadata.TokenEndpoint == "" || (server.AuthType == model.MCPAuthTypeOAuthAuthorizationCode && metadata.AuthorizationEndpoint == "") {
		return OAuthMetadata{}, validationErrorf("OAuth metadata is missing required endpoints")
	}
	if err := validateOAuthMetadataEndpoints(metadata); err != nil {
		return OAuthMetadata{}, err
	}
	result := s.db.WithContext(ctx).Model(&model.MCPServer{}).
		Where("id = ? AND org_id = ?", server.ID, server.OrgID).
		Update("oauth_metadata", encodeOAuthMetadata(metadata))
	if result.Error != nil {
		return OAuthMetadata{}, fmt.Errorf("cache OAuth metadata: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return OAuthMetadata{}, ErrNotFound
	}
	return metadata, nil
}
