package mcpservers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func (s *Service) UpsertAuthorization(ctx context.Context, orgID, serverID, actorUserID uuid.UUID, input AuthorizationInput) (*AuthorizationSummary, error) {
	server, err := s.GetServer(ctx, orgID, serverID, &actorUserID)
	if err != nil {
		return nil, err
	}
	if err := s.upsertAuthorization(ctx, s.db, *server, actorUserID, input); err != nil {
		return nil, err
	}
	row, err := s.getAuthorization(ctx, orgID, serverID, input.PrincipalType, actorUserID)
	if err != nil {
		return nil, err
	}
	summary, err := s.authorizationSummary(*row)
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func (s *Service) upsertAuthorization(ctx context.Context, db *gorm.DB, server model.MCPServer, actorUserID uuid.UUID, input AuthorizationInput) error {
	principal, principalUserID, err := normalizePrincipal(server, actorUserID, input.PrincipalType)
	if err != nil {
		return err
	}
	input.PrincipalType = principal
	envelope, err := normalizeCredentialEnvelope(server.AuthType, input)
	if err != nil {
		return err
	}
	encrypted, err := s.encryptEnvelope(envelope)
	if err != nil {
		return err
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = model.MCPAuthorizationStatusActive
	}
	if status != model.MCPAuthorizationStatusActive && status != model.MCPAuthorizationStatusExpired && status != model.MCPAuthorizationStatusRevoked {
		return validationErrorf("authorization status must be active, expired, or revoked")
	}
	row := model.MCPAuthorization{
		ID: uuid.New(), OrgID: server.OrgID, MCPServerID: server.ID,
		PrincipalType: principal, PrincipalUserID: principalUserID,
		AuthType: server.AuthType, CredentialsEncrypted: encrypted,
		ClientID: strings.TrimSpace(input.ClientID), Scopes: pq.StringArray(sortedUnique(input.Scopes)),
		TokenType: strings.TrimSpace(input.TokenType), ExpiresAt: input.ExpiresAt,
		RefreshExpiresAt: input.RefreshExpiresAt, Status: status,
	}
	query := db.WithContext(ctx).Model(&model.MCPAuthorization{}).
		Where("org_id = ? AND mcp_server_id = ? AND principal_type = ?", server.OrgID, server.ID, principal)
	if principalUserID != nil {
		query = query.Where("principal_user_id = ?", *principalUserID)
	} else {
		query = query.Where("principal_user_id IS NULL")
	}
	assignments := map[string]any{
		"auth_type": server.AuthType, "credentials_encrypted": encrypted,
		"client_id": row.ClientID, "scopes": row.Scopes, "token_type": row.TokenType,
		"expires_at": row.ExpiresAt, "refresh_expires_at": row.RefreshExpiresAt,
		"status": row.Status, "updated_at": time.Now().UTC(),
	}
	result := query.Updates(assignments)
	if result.Error != nil {
		return fmt.Errorf("update mcp authorization: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return nil
	}
	if err := db.WithContext(ctx).Create(&row).Error; err != nil {
		if duplicateKey(err) {
			return ErrConflict
		}
		return fmt.Errorf("create mcp authorization: %w", err)
	}
	return nil
}

func normalizePrincipal(server model.MCPServer, actorUserID uuid.UUID, raw string) (string, *uuid.UUID, error) {
	principal := strings.TrimSpace(raw)
	if principal == "" {
		if server.Scope == model.MCPServerScopePersonal || server.AuthorizationPolicy == model.MCPAuthorizationPolicyUserRequired {
			principal = model.MCPPrincipalUser
		} else {
			principal = model.MCPPrincipalOrgService
		}
	}
	if principal == model.MCPPrincipalUser {
		if server.AuthType == model.MCPAuthTypeOAuthClientCredentials || server.AuthorizationPolicy == model.MCPAuthorizationPolicyServiceRequired {
			return "", nil, validationErrorf("this MCP server requires an organization service authorization")
		}
		if actorUserID == uuid.Nil {
			return "", nil, validationErrorf("user authorization requires a user")
		}
		if server.Scope == model.MCPServerScopePersonal && (server.OwnerUserID == nil || *server.OwnerUserID != actorUserID) {
			return "", nil, ErrNotFound
		}
		return principal, &actorUserID, nil
	}
	if principal != model.MCPPrincipalOrgService {
		return "", nil, validationErrorf("principal_type must be user or org_service")
	}
	if server.Scope == model.MCPServerScopePersonal {
		return "", nil, validationErrorf("personal MCP servers cannot use an organization service authorization")
	}
	if server.AuthorizationPolicy == model.MCPAuthorizationPolicyUserRequired && server.AuthType != model.MCPAuthTypeOAuthAuthorizationCode {
		return "", nil, validationErrorf("this MCP server requires a user authorization")
	}
	return principal, nil, nil
}

func normalizeCredentialEnvelope(authType string, input AuthorizationInput) (credentialEnvelope, error) {
	switch authType {
	case model.MCPAuthTypeNone:
		return credentialEnvelope{}, nil
	case model.MCPAuthTypeStaticBearer:
		if strings.TrimSpace(input.BearerToken) == "" || !validHeaderValue(input.BearerToken) {
			return credentialEnvelope{}, validationErrorf("bearer_token is required")
		}
		return credentialEnvelope{BearerToken: input.BearerToken}, nil
	case model.MCPAuthTypeStaticHeader:
		if strings.TrimSpace(input.HeaderValue) == "" || !validHeaderValue(input.HeaderValue) {
			return credentialEnvelope{}, validationErrorf("header_value is required")
		}
		return credentialEnvelope{HeaderValue: input.HeaderValue}, nil
	case model.MCPAuthTypeOAuthAuthorizationCode:
		if strings.TrimSpace(input.ClientID) == "" {
			return credentialEnvelope{}, validationErrorf("client_id is required")
		}
		if input.TokenType != "" && !strings.EqualFold(strings.TrimSpace(input.TokenType), "Bearer") {
			return credentialEnvelope{}, validationErrorf("token_type must be Bearer")
		}
		if !validHeaderValue(input.AccessToken) {
			return credentialEnvelope{}, validationErrorf("access_token is invalid")
		}
		return credentialEnvelope{AccessToken: input.AccessToken, RefreshToken: input.RefreshToken, ClientSecret: input.ClientSecret}, nil
	case model.MCPAuthTypeOAuthClientCredentials:
		if strings.TrimSpace(input.ClientID) == "" || strings.TrimSpace(input.ClientSecret) == "" {
			return credentialEnvelope{}, validationErrorf("client_id and client_secret are required")
		}
		if input.TokenType != "" && !strings.EqualFold(strings.TrimSpace(input.TokenType), "Bearer") {
			return credentialEnvelope{}, validationErrorf("token_type must be Bearer")
		}
		if !validHeaderValue(input.AccessToken) {
			return credentialEnvelope{}, validationErrorf("access_token is invalid")
		}
		return credentialEnvelope{AccessToken: input.AccessToken, ClientSecret: input.ClientSecret}, nil
	default:
		return credentialEnvelope{}, validationErrorf("unsupported auth_type")
	}
}

func validHeaderValue(value string) bool { return !strings.ContainsAny(value, "\r\n") }

func (s *Service) DeleteAuthorization(ctx context.Context, orgID, serverID, actorUserID uuid.UUID, principalType string) error {
	server, err := s.GetServer(ctx, orgID, serverID, &actorUserID)
	if err != nil {
		return err
	}
	principal, principalUserID, err := normalizePrincipal(*server, actorUserID, principalType)
	if err != nil {
		return err
	}
	query := s.db.WithContext(ctx).Where("org_id = ? AND mcp_server_id = ? AND principal_type = ?", orgID, serverID, principal)
	if principalUserID != nil {
		query = query.Where("principal_user_id = ?", *principalUserID)
	} else {
		query = query.Where("principal_user_id IS NULL")
	}
	result := query.Delete(&model.MCPAuthorization{})
	if result.Error != nil {
		return fmt.Errorf("delete mcp authorization: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrAuthorizationNotFound
	}
	return nil
}

func (s *Service) AuthorizationSummaries(ctx context.Context, orgID, serverID uuid.UUID, actorUserID *uuid.UUID, includeService bool) (*AuthorizationSummary, *AuthorizationSummary, error) {
	summaries, err := s.AuthorizationSummariesForServers(ctx, orgID, []uuid.UUID{serverID}, actorUserID, includeService)
	if err != nil {
		return nil, nil, err
	}
	summary := summaries[serverID]
	return summary.User, summary.Service, nil
}

func (s *Service) AuthorizationSummariesForServers(ctx context.Context, orgID uuid.UUID, serverIDs []uuid.UUID, actorUserID *uuid.UUID, includeService bool) (map[uuid.UUID]AuthorizationSummaries, error) {
	summaries := make(map[uuid.UUID]AuthorizationSummaries, len(serverIDs))
	if len(serverIDs) == 0 {
		return summaries, nil
	}
	var rows []model.MCPAuthorization
	query := s.db.WithContext(ctx).Where("org_id = ? AND mcp_server_id IN ?", orgID, serverIDs)
	if actorUserID == nil {
		query = query.Where("principal_type = ?", model.MCPPrincipalOrgService)
	} else if !includeService {
		query = query.Where("principal_type = ? AND principal_user_id = ?", model.MCPPrincipalUser, *actorUserID)
	} else {
		query = query.Where("(principal_type = ? AND principal_user_id = ?) OR principal_type = ?", model.MCPPrincipalUser, *actorUserID, model.MCPPrincipalOrgService)
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list mcp authorizations: %w", err)
	}
	for _, row := range rows {
		summary, err := s.authorizationSummary(row)
		if err != nil {
			return nil, err
		}
		serverSummaries := summaries[row.MCPServerID]
		if row.PrincipalType == model.MCPPrincipalUser {
			serverSummaries.User = &summary
		} else {
			serverSummaries.Service = &summary
		}
		summaries[row.MCPServerID] = serverSummaries
	}
	return summaries, nil
}

func (s *Service) authorizationSummary(row model.MCPAuthorization) (AuthorizationSummary, error) {
	status := row.Status
	if row.AuthType == model.MCPAuthTypeOAuthAuthorizationCode && row.Status == model.MCPAuthorizationStatusActive {
		envelope, err := s.decryptEnvelope(row.CredentialsEncrypted)
		if err != nil {
			return AuthorizationSummary{}, err
		}
		if envelope.AccessToken == "" {
			status = "pending"
		}
	}
	return AuthorizationSummary{
		ID: row.ID, PrincipalType: row.PrincipalType, PrincipalUserID: row.PrincipalUserID,
		AuthType: row.AuthType, Configured: len(row.CredentialsEncrypted) > 0,
		ClientID: row.ClientID, Scopes: append([]string{}, row.Scopes...), TokenType: row.TokenType,
		ExpiresAt: row.ExpiresAt, RefreshExpiresAt: row.RefreshExpiresAt, Status: status,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

func (s *Service) getAuthorization(ctx context.Context, orgID, serverID uuid.UUID, principalType string, userID uuid.UUID) (*model.MCPAuthorization, error) {
	var row model.MCPAuthorization
	query := s.db.WithContext(ctx).Where("org_id = ? AND mcp_server_id = ? AND principal_type = ? AND status = ?", orgID, serverID, principalType, model.MCPAuthorizationStatusActive)
	if principalType == model.MCPPrincipalUser {
		query = query.Where("principal_user_id = ?", userID)
	} else {
		query = query.Where("principal_user_id IS NULL")
	}
	if err := query.First(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAuthorizationNotFound
	} else if err != nil {
		return nil, fmt.Errorf("load mcp authorization: %w", err)
	}
	return &row, nil
}

func authorizationNeedsRefresh(row model.MCPAuthorization, envelope credentialEnvelope, now time.Time) bool {
	if envelope.AccessToken == "" {
		return true
	}
	return row.ExpiresAt != nil && !row.ExpiresAt.After(now.Add(30*time.Second))
}
