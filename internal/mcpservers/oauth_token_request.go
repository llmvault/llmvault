package mcpservers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

func (s *Service) ensureAccessToken(ctx context.Context, server model.MCPServer, authorization model.MCPAuthorization) (*model.MCPAuthorization, credentialEnvelope, error) {
	envelope, err := s.decryptEnvelope(authorization.CredentialsEncrypted)
	if err != nil {
		return nil, credentialEnvelope{}, err
	}
	if !authorizationNeedsRefresh(authorization, envelope, s.now()) {
		return &authorization, envelope, nil
	}
	if server.AuthType != model.MCPAuthTypeOAuthAuthorizationCode && server.AuthType != model.MCPAuthTypeOAuthClientCredentials {
		return &authorization, envelope, nil
	}
	var refreshed model.MCPAuthorization
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND org_id = ?", authorization.ID, server.OrgID).First(&refreshed).Error; err != nil {
			return fmt.Errorf("lock mcp authorization: %w", err)
		}
		lockedEnvelope, err := s.decryptEnvelope(refreshed.CredentialsEncrypted)
		if err != nil {
			return err
		}
		if !authorizationNeedsRefresh(refreshed, lockedEnvelope, s.now()) {
			return nil
		}
		metadata, err := s.DiscoverOAuth(ctx, server)
		if err != nil {
			return err
		}
		values := url.Values{"client_id": {refreshed.ClientID}}
		if resource := firstNonEmpty(metadata.Resource, server.URL); resource != "" {
			values.Set("resource", resource)
		}
		if len(refreshed.Scopes) > 0 {
			values.Set("scope", strings.Join(refreshed.Scopes, " "))
		}
		if server.AuthType == model.MCPAuthTypeOAuthAuthorizationCode {
			if lockedEnvelope.RefreshToken == "" {
				return ErrAuthorizationNotFound
			}
			values.Set("grant_type", "refresh_token")
			values.Set("refresh_token", lockedEnvelope.RefreshToken)
		} else {
			values.Set("grant_type", "client_credentials")
		}
		token, err := s.requestToken(ctx, metadata, refreshed.ClientID, lockedEnvelope.ClientSecret, values)
		if err != nil {
			return err
		}
		updated, err := mergeToken(refreshed, lockedEnvelope, token, s.now())
		if err != nil {
			return err
		}
		encrypted, err := s.encryptEnvelope(updated.envelope)
		if err != nil {
			return err
		}
		result := tx.WithContext(ctx).Model(&model.MCPAuthorization{}).
			Where("id = ? AND org_id = ?", refreshed.ID, server.OrgID).
			Updates(map[string]any{"credentials_encrypted": encrypted, "token_type": updated.tokenType, "scopes": pq.StringArray(updated.scopes), "expires_at": updated.expiresAt, "status": model.MCPAuthorizationStatusActive})
		if result.Error != nil {
			return fmt.Errorf("refresh mcp authorization: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrAuthorizationNotFound
		}
		refreshed.CredentialsEncrypted = encrypted
		refreshed.TokenType = updated.tokenType
		refreshed.Scopes = pq.StringArray(updated.scopes)
		refreshed.ExpiresAt = updated.expiresAt
		return nil
	})
	if err != nil {
		return nil, credentialEnvelope{}, err
	}
	refreshedEnvelope, err := s.decryptEnvelope(refreshed.CredentialsEncrypted)
	if err != nil {
		return nil, credentialEnvelope{}, err
	}
	return &refreshed, refreshedEnvelope, nil
}

func (s *Service) requestToken(ctx context.Context, metadata OAuthMetadata, clientID, clientSecret string, values url.Values) (oauthTokenResponse, error) {
	endpoint := metadata.TokenEndpoint
	if strings.TrimSpace(endpoint) == "" {
		return oauthTokenResponse{}, validationErrorf("OAuth token endpoint is missing")
	}
	validated, err := normalizeEndpointURL(endpoint)
	if err != nil {
		return oauthTokenResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, validated, strings.NewReader(values.Encode()))
	if err != nil {
		return oauthTokenResponse{}, fmt.Errorf("create OAuth token request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	authMethod, err := selectTokenEndpointAuthMethod(metadata.TokenEndpointAuthMethodsSupported, clientSecret != "")
	if err != nil {
		return oauthTokenResponse{}, err
	}
	switch authMethod {
	case "client_secret_basic":
		request.SetBasicAuth(clientID, clientSecret)
	case "client_secret_post":
		values.Set("client_secret", clientSecret)
		request.Body = io.NopCloser(strings.NewReader(values.Encode()))
		request.ContentLength = int64(len(values.Encode()))
	}
	response, err := s.httpClient.Do(request)
	if err != nil {
		return oauthTokenResponse{}, fmt.Errorf("request OAuth token: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.CopyN(io.Discard, response.Body, maxOAuthResponseBytes)
		return oauthTokenResponse{}, fmt.Errorf("OAuth token endpoint returned status %d", response.StatusCode)
	}
	var token oauthTokenResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxOAuthResponseBytes))
	if err := decoder.Decode(&token); err != nil {
		return oauthTokenResponse{}, fmt.Errorf("decode OAuth token response: %w", err)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return oauthTokenResponse{}, validationErrorf("OAuth token response has no access_token")
	}
	if !validHeaderValue(token.AccessToken) {
		return oauthTokenResponse{}, validationErrorf("OAuth token response has an invalid access_token")
	}
	return token, nil
}

func selectTokenEndpointAuthMethod(supported []string, hasClientSecret bool) (string, error) {
	if len(supported) == 0 {
		if hasClientSecret {
			return "client_secret_basic", nil
		}
		return "none", nil
	}
	methods := make(map[string]bool, len(supported))
	for _, method := range supported {
		methods[strings.TrimSpace(method)] = true
	}
	if hasClientSecret {
		if methods["client_secret_basic"] {
			return "client_secret_basic", nil
		}
		if methods["client_secret_post"] {
			return "client_secret_post", nil
		}
	}
	if methods["none"] {
		return "none", nil
	}
	return "", validationErrorf("OAuth token endpoint does not support this client's authentication method")
}

func (s *Service) registerOAuthClient(ctx context.Context, endpoint string, scopes []string) (dynamicClientRegistrationResponse, error) {
	validated, err := normalizeEndpointURL(endpoint)
	if err != nil {
		return dynamicClientRegistrationResponse{}, err
	}
	payload := map[string]any{
		"client_name":    "Hivy MCP Client",
		"redirect_uris":  []string{s.callbackURL},
		"grant_types":    []string{"authorization_code", "refresh_token"},
		"response_types": []string{"code"},
	}
	if len(scopes) > 0 {
		payload["scope"] = strings.Join(scopes, " ")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return dynamicClientRegistrationResponse{}, fmt.Errorf("encode OAuth client registration: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, validated, strings.NewReader(string(raw)))
	if err != nil {
		return dynamicClientRegistrationResponse{}, fmt.Errorf("create OAuth client registration request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := s.httpClient.Do(request)
	if err != nil {
		return dynamicClientRegistrationResponse{}, fmt.Errorf("register OAuth client: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.CopyN(io.Discard, response.Body, maxOAuthResponseBytes)
		return dynamicClientRegistrationResponse{}, fmt.Errorf("OAuth client registration returned status %d", response.StatusCode)
	}
	var registered dynamicClientRegistrationResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, maxOAuthResponseBytes)).Decode(&registered); err != nil {
		return dynamicClientRegistrationResponse{}, fmt.Errorf("decode OAuth client registration: %w", err)
	}
	if strings.TrimSpace(registered.ClientID) == "" {
		return dynamicClientRegistrationResponse{}, validationErrorf("OAuth client registration returned no client_id")
	}
	return registered, nil
}
