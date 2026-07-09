package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

type externalChannelAutoCreateRequest struct {
	Provider       string
	Connection     model.Connection
	ResourceType   string
	ResourceKey    string
	ResourceName   string
	DefaultAgentID uuid.UUID
	Metadata       model.JSON
}

func findOrAutoCreateExternalChannel(ctx context.Context, db *gorm.DB, provisioner ChannelExternalProvisioner, orgID uuid.UUID, req externalChannelAutoCreateRequest) (model.Channel, error) {
	channel, found, err := findExternalChannel(ctx, db, orgID, req)
	if err != nil || found {
		return channel, err
	}
	var agent model.Agent
	if err := db.WithContext(ctx).Select("team_id").
		Where("id = ? AND org_id = ?", req.DefaultAgentID, orgID).
		First(&agent).Error; err != nil {
		return model.Channel{}, fmt.Errorf("load external channel default agent team: %w", err)
	}
	channel = buildExternalChannel(orgID, req, agent.TeamID)
	if channel.Name == "" {
		return model.Channel{}, fmt.Errorf("external resource name is required")
	}
	if err := provisionExternalChannel(ctx, provisioner, orgID, req); err != nil {
		return model.Channel{}, err
	}
	result := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&channel)
	if result.Error != nil {
		return model.Channel{}, fmt.Errorf("create external channel: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		return channel, nil
	}
	channel, found, err = findExternalChannel(ctx, db, orgID, req)
	if err != nil {
		return model.Channel{}, err
	}
	if found {
		return channel, nil
	}
	return model.Channel{}, fmt.Errorf("external channel create conflicted but no channel was found")
}

func findExternalChannel(ctx context.Context, db *gorm.DB, orgID uuid.UUID, req externalChannelAutoCreateRequest) (model.Channel, bool, error) {
	var channel model.Channel
	err := db.WithContext(ctx).
		Where("org_id = ? AND archived_at IS NULL", orgID).
		Where("origin = ? AND external_provider = ?", "external", req.Provider).
		Where("external_connection_id = ? AND external_resource_type = ?", req.Connection.ID, req.ResourceType).
		Where("external_resource_key = ?", req.ResourceKey).
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Channel{}, false, nil
	}
	if err != nil {
		return model.Channel{}, false, fmt.Errorf("load external channel: %w", err)
	}
	return channel, true, nil
}

func buildExternalChannel(orgID uuid.UUID, req externalChannelAutoCreateRequest, teamID uuid.UUID) model.Channel {
	displayName := strings.TrimSpace(req.ResourceName)
	if displayName == "" {
		displayName = req.ResourceKey
	}
	metadata := req.Metadata
	if metadata == nil {
		metadata = model.JSON{}
	}
	return model.Channel{
		OrgID:                orgID,
		Name:                 providerPrefixedChannelName(req.Provider, displayName),
		Kind:                 "standard",
		Visibility:           "public",
		TeamID:               teamID,
		DefaultAgentID:       req.DefaultAgentID,
		Origin:               "external",
		ExternalProvider:     req.Provider,
		ExternalConnectionID: &req.Connection.ID,
		ExternalWorkspaceKey: req.Connection.NangoConnectionID,
		ExternalResourceType: req.ResourceType,
		ExternalResourceKey:  req.ResourceKey,
		ExternalResourceName: displayName,
		ExternalMetadata:     metadata,
	}
}

func provisionExternalChannel(ctx context.Context, provisioner ChannelExternalProvisioner, orgID uuid.UUID, req externalChannelAutoCreateRequest) error {
	if provisioner == nil {
		return nil
	}
	return provisioner.EnsureExternalChannel(ctx, orgID, ChannelExternalProvisionRequest{
		Provider:     req.Provider,
		ConnectionID: req.Connection.ID,
		WorkspaceKey: req.Connection.NangoConnectionID,
		ResourceType: req.ResourceType,
		ResourceKey:  req.ResourceKey,
		ResourceName: req.ResourceName,
	})
}
