package handler_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
	"gorm.io/gorm"
)

func firstTeamID(t *testing.T, db *gorm.DB, orgID uuid.UUID) uuid.UUID {
	t.Helper()
	var team model.Team
	if db.Where("org_id = ?", orgID).Order("created_at ASC").First(&team).Error == nil {
		return team.ID
	}
	team = model.Team{ID: uuid.New(), OrgID: orgID, Name: "seed-team-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	return team.ID
}
func seedDefaultModelCredential(t *testing.T, db *gorm.DB) {
	t.Helper()
	cred := model.Credential{ID: uuid.New(), Label: "runtime-" + uuid.NewString()[:8], BaseURL: "https://api.atlascloud.ai/v1", AuthScheme: "bearer", ProviderID: "atlascloud", EncryptedKey: []byte("enc"), WrappedDEK: []byte("dek")}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })
}

func grantManagedConnectionToAgentTeam(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID, provider string) {
	t.Helper()
	var agent model.Agent
	if err := db.Where("id = ? AND org_id = ?", agentID, orgID).First(&agent).Error; err != nil {
		t.Fatalf("load agent team: %v", err)
	}
	var connection model.Connection
	if err := db.Table("connections").
		Joins("JOIN integrations ON integrations.id = connections.integration_id").
		Where("connections.org_id = ? AND integrations.provider = ? AND connections.revoked_at IS NULL", orgID, provider).
		Order("connections.created_at DESC").
		First(&connection).Error; err != nil {
		t.Fatalf("load %s connection: %v", provider, err)
	}
	grant := model.TeamConnectionGrant{
		ID:           uuid.New(),
		OrgID:        orgID,
		TeamID:       agent.TeamID,
		ConnectionID: &connection.ID,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatalf("grant %s connection to team: %v", provider, err)
	}
}

func grantDatabaseConnectionToAgentTeam(t *testing.T, db *gorm.DB, orgID, agentID, connectionID uuid.UUID) {
	t.Helper()
	var agent model.Agent
	if err := db.Where("id = ? AND org_id = ?", agentID, orgID).First(&agent).Error; err != nil {
		t.Fatalf("load agent team: %v", err)
	}
	grant := model.TeamConnectionGrant{
		ID:                   uuid.New(),
		OrgID:                orgID,
		TeamID:               agent.TeamID,
		DatabaseConnectionID: &connectionID,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatalf("grant database connection to team: %v", err)
	}
}

func createDatabaseScopeTestOrg(t *testing.T, db *gorm.DB) model.Org {
	t.Helper()
	org := model.Org{
		ID:        uuid.New(),
		Name:      fmt.Sprintf("database-scope-%s", uuid.NewString()[:8]),
		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	return org
}

func createDatabaseScopeTestAgent(t *testing.T, db *gorm.DB, orgID uuid.UUID, label string) model.Agent {
	t.Helper()
	team := model.Team{ID: uuid.New(), OrgID: orgID, Name: "database-scope-" + label + "-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	agent := model.Agent{
		ID:     uuid.New(),
		OrgID:  &orgID,
		TeamID: team.ID,
		Name:   "database-scope-" + label + "-" + uuid.NewString()[:8],
		Model:  "test-model",
		Status: "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return agent
}

func createDatabaseScopeTestSandbox(t *testing.T, db *gorm.DB, encKey *crypto.SymmetricKey, orgID, agentID uuid.UUID) string {
	t.Helper()
	runtimeSecret := "database-scope-runtime-" + uuid.NewString()
	encryptedRuntimeSecret, err := encKey.EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sandbox := model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  &orgID,
		AgentID:                &agentID,
		EncryptedRuntimeSecret: encryptedRuntimeSecret,
		Status:                 "running",
		ExternalID:             "database-scope-" + uuid.NewString(),
		RuntimeURL:             "http://localhost:25434",
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	return runtimeSecret
}
