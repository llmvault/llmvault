package handler_test

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func installTestPluginAccess(t *testing.T, db *gorm.DB, orgID uuid.UUID, agentID uuid.UUID, provider string) uuid.UUID {
	t.Helper()
	pluginID := uuid.New()
	plugin := model.Plugin{
		ID:     pluginID,
		Slug:   provider + "-test-" + uuid.New().String()[:8],
		Name:   provider,
		Status: model.PluginStatusActive,
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create test plugin: %v", err)
	}
	if err := db.Create(&model.PluginIntegration{
		PluginID: pluginID,
		Provider: provider,
		Kind:     model.PluginIntegrationKindIntegration,
		Required: true,
	}).Error; err != nil {
		t.Fatalf("create test plugin integration: %v", err)
	}
	if err := db.Create(&model.OrgPluginInstall{
		ID:       uuid.New(),
		OrgID:    orgID,
		PluginID: pluginID,
	}).Error; err != nil {
		t.Fatalf("create test org plugin install: %v", err)
	}
	grantPluginToAgentTeam(t, db, orgID, agentID, pluginID)
	t.Cleanup(func() {
		db.Where("org_id = ? AND plugin_id = ?", orgID, pluginID).Delete(&model.TeamPlugin{})
		db.Where("org_id = ? AND plugin_id = ?", orgID, pluginID).Delete(&model.OrgPluginInstall{})
		db.Where("plugin_id = ?", pluginID).Delete(&model.PluginIntegration{})
		db.Where("id = ?", pluginID).Delete(&model.Plugin{})
	})
	return pluginID
}
