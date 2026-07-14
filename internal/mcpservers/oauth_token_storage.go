package mcpservers

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/model"
)

func (s *Service) ClientMetadataURL() string {
	parsed, err := url.Parse(s.callbackURL)
	if err != nil || parsed.Host == "" {
		return ""
	}
	if strings.HasSuffix(parsed.Path, "/callback") {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/callback") + "/client-metadata"
	} else {
		parsed.Path = strings.TrimRight(parsed.Path, "/") + "/client-metadata"
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func (s *Service) ClientMetadata() OAuthClientMetadata {
	return OAuthClientMetadata{
		ClientID: s.ClientMetadataURL(), ClientName: "Hivy MCP Client",
		RedirectURIs: []string{s.callbackURL}, GrantTypes: []string{"authorization_code", "refresh_token"},
		ResponseTypes: []string{"code"}, TokenEndpointAuthMethod: "none",
	}
}

type mergedToken struct {
	envelope  credentialEnvelope
	tokenType string
	scopes    []string
	expiresAt *time.Time
}

func mergeToken(row model.MCPAuthorization, current credentialEnvelope, token oauthTokenResponse, now time.Time) (mergedToken, error) {
	current.AccessToken = token.AccessToken
	if token.RefreshToken != "" {
		current.RefreshToken = token.RefreshToken
	}
	tokenType := strings.TrimSpace(token.TokenType)
	if tokenType == "" {
		tokenType = firstNonEmpty(row.TokenType, "Bearer")
	}
	if !strings.EqualFold(tokenType, "Bearer") {
		return mergedToken{}, validationErrorf("OAuth token_type must be Bearer")
	}
	tokenType = "Bearer"
	scopes := token.Scopes
	if len(scopes) == 0 && token.Scope != "" {
		scopes = strings.Fields(token.Scope)
	}
	if len(scopes) == 0 {
		scopes = append([]string{}, row.Scopes...)
	}
	var expiresAt *time.Time
	if token.ExpiresIn > 0 {
		value := now.Add(time.Duration(token.ExpiresIn) * time.Second)
		expiresAt = &value
	}
	return mergedToken{envelope: current, tokenType: tokenType, scopes: sortedUnique(scopes), expiresAt: expiresAt}, nil
}

func (s *Service) saveToken(ctx context.Context, row *model.MCPAuthorization, current credentialEnvelope, token oauthTokenResponse) error {
	updated, err := mergeToken(*row, current, token, s.now())
	if err != nil {
		return err
	}
	encrypted, err := s.encryptEnvelope(updated.envelope)
	if err != nil {
		return err
	}
	result := s.db.WithContext(ctx).Model(&model.MCPAuthorization{}).
		Where("id = ? AND org_id = ?", row.ID, row.OrgID).
		Updates(map[string]any{"credentials_encrypted": encrypted, "token_type": updated.tokenType, "scopes": pq.StringArray(updated.scopes), "expires_at": updated.expiresAt, "status": model.MCPAuthorizationStatusActive})
	if result.Error != nil {
		return fmt.Errorf("store OAuth token: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrAuthorizationNotFound
	}
	return nil
}
