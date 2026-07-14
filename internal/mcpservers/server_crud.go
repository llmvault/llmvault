package mcpservers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func (s *Service) ListServers(ctx context.Context, orgID uuid.UUID, actorUserID *uuid.UUID, input ListServersInput) (ListServersResult, error) {
	limit := input.Limit
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	servers := []model.MCPServer{}
	query := s.db.WithContext(ctx).Where("org_id = ?", orgID)
	if actorUserID == nil {
		query = query.Where("scope = ?", model.MCPServerScopeOrg)
	} else if !input.IncludeOrg {
		query = query.Where("scope = ? AND owner_user_id = ?", model.MCPServerScopePersonal, *actorUserID)
	} else {
		query = query.Where("scope = ? OR (scope = ? AND owner_user_id = ?)", model.MCPServerScopeOrg, model.MCPServerScopePersonal, *actorUserID)
	}
	if input.BeforeCreatedAt != nil && input.BeforeID != nil {
		query = query.Where("(created_at, id) < (?, ?)", *input.BeforeCreatedAt, *input.BeforeID)
	}
	if err := query.Order("created_at DESC, id DESC").Limit(limit + 1).Find(&servers).Error; err != nil {
		return ListServersResult{}, fmt.Errorf("list mcp servers: %w", err)
	}
	hasMore := len(servers) > limit
	if hasMore {
		servers = servers[:limit]
	}
	return ListServersResult{Servers: servers, HasMore: hasMore}, nil
}

func (s *Service) GetServer(ctx context.Context, orgID, serverID uuid.UUID, actorUserID *uuid.UUID) (*model.MCPServer, error) {
	var server model.MCPServer
	query := s.db.WithContext(ctx).Where("id = ? AND org_id = ?", serverID, orgID)
	if actorUserID == nil {
		query = query.Where("scope = ?", model.MCPServerScopeOrg)
	} else {
		query = query.Where("scope = ? OR owner_user_id = ?", model.MCPServerScopeOrg, *actorUserID)
	}
	if err := query.First(&server).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("load mcp server: %w", err)
	}
	return &server, nil
}

func (s *Service) UpdateServer(ctx context.Context, orgID, serverID uuid.UUID, actorUserID *uuid.UUID, params UpdateServerParams) (*model.MCPServer, error) {
	server, err := s.GetServer(ctx, orgID, serverID, actorUserID)
	if err != nil {
		return nil, err
	}
	updates, err := normalizeServerUpdates(*server, params)
	if err != nil {
		return nil, err
	}
	if len(updates) > 0 {
		authTypeChanged := params.AuthType != nil && strings.TrimSpace(*params.AuthType) != server.AuthType
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			result := tx.WithContext(ctx).Model(&model.MCPServer{}).
				Where("id = ? AND org_id = ?", serverID, orgID).Updates(updates)
			if result.Error != nil {
				if duplicateKey(result.Error) {
					return ErrConflict
				}
				return fmt.Errorf("update mcp server: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return ErrNotFound
			}
			if authTypeChanged {
				if err := tx.WithContext(ctx).Where("org_id = ? AND mcp_server_id = ?", orgID, serverID).
					Delete(&model.MCPAuthorization{}).Error; err != nil {
					return fmt.Errorf("clear stale mcp authorizations: %w", err)
				}
				if err := tx.WithContext(ctx).Where("org_id = ? AND mcp_server_id = ?", orgID, serverID).
					Delete(&model.MCPOAuthState{}).Error; err != nil {
					return fmt.Errorf("clear stale mcp oauth states: %w", err)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return s.GetServer(ctx, orgID, serverID, actorUserID)
}

func normalizeServerUpdates(server model.MCPServer, params UpdateServerParams) (map[string]any, error) {
	merged := CreateServerParams{
		Scope: server.Scope, Name: server.Name, Slug: server.Slug, Description: server.Description,
		URL: server.URL, Transport: server.Transport, AuthType: server.AuthType,
		AuthorizationPolicy: server.AuthorizationPolicy, HeaderName: server.HeaderName,
		OAuthMetadata: decodeOAuthMetadata(server.OAuthMetadata),
	}
	if params.Name != nil {
		merged.Name = *params.Name
	}
	if params.Slug != nil {
		merged.Slug = *params.Slug
	}
	if params.Description != nil {
		merged.Description = *params.Description
	}
	if params.URL != nil {
		merged.URL = *params.URL
	}
	if params.Transport != nil {
		merged.Transport = *params.Transport
	}
	if params.AuthType != nil {
		merged.AuthType = *params.AuthType
	}
	if params.AuthorizationPolicy != nil {
		merged.AuthorizationPolicy = *params.AuthorizationPolicy
	}
	if params.HeaderName != nil {
		merged.HeaderName = *params.HeaderName
	}
	if params.OAuthMetadata != nil {
		merged.OAuthMetadata = *params.OAuthMetadata
	}
	actorID := uuid.Nil
	if server.OwnerUserID != nil {
		actorID = *server.OwnerUserID
	}
	normalized, err := normalizeServer(merged, server.OrgID, actorID)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{
		"name": normalized.Name, "slug": normalized.Slug, "description": normalized.Description,
		"url": normalized.URL, "transport": normalized.Transport, "auth_type": normalized.AuthType,
		"authorization_policy": normalized.AuthorizationPolicy, "header_name": normalized.HeaderName,
		"oauth_metadata": normalized.OAuthMetadata,
	}
	if params.Status != nil {
		status := strings.TrimSpace(*params.Status)
		if status != model.MCPServerStatusActive && status != model.MCPServerStatusDisabled {
			return nil, validationErrorf("status must be active or disabled")
		}
		updates["status"] = status
	}
	return updates, nil
}

func (s *Service) DeleteServer(ctx context.Context, orgID, serverID uuid.UUID, actorUserID *uuid.UUID) error {
	server, err := s.GetServer(ctx, orgID, serverID, actorUserID)
	if err != nil {
		return err
	}
	result := s.db.WithContext(ctx).Where("id = ? AND org_id = ?", server.ID, orgID).Delete(&model.MCPServer{})
	if result.Error != nil {
		return fmt.Errorf("delete mcp server: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrNotFound
	}
	return nil
}
