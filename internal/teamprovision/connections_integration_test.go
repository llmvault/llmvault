package teamprovision

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

// TestRequiredCatalogConnectionGrantLifecycle persists the entire grant
// lifecycle and verifies that active catalog agents protect required provider
// access without leaking a same-provider connection across orgs.
func TestRequiredCatalogConnectionGrantLifecycle(t *testing.T) {
	db, err := gorm.Open(postgres.Open(testdb.DatabaseURL()), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	testdb.ApplyMigrations(t, db)

	org := model.Org{ID: uuid.New(), Name: "required-connection-" + uuid.NewString()[:8], Active: true}
	otherOrg := model.Org{ID: uuid.New(), Name: "other-connection-" + uuid.NewString()[:8], Active: true}
	user := model.User{ID: uuid.New(), Email: "required-connection-" + uuid.NewString() + "@example.test"}
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "Engineering"}
	for _, row := range []any{&org, &otherOrg, &user, &team} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}

	integration := model.Integration{ID: uuid.New(), UniqueKey: "github-app-" + uuid.NewString(), Provider: "github-app", DisplayName: "GitHub"}
	if err := db.Create(&integration).Error; err != nil {
		t.Fatalf("seed integration: %v", err)
	}
	connection := model.Connection{ID: uuid.New(), OrgID: org.ID, UserID: user.ID, IntegrationID: integration.ID, NangoConnectionID: "required"}
	foreignConnection := model.Connection{ID: uuid.New(), OrgID: otherOrg.ID, UserID: user.ID, IntegrationID: integration.ID, NangoConnectionID: "foreign"}
	for _, row := range []*model.Connection{&connection, &foreignConnection} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed connection: %v", err)
		}
	}

	if err := GrantConnection(t.Context(), db, org.ID, team.ID, foreignConnection.ID, &user.ID); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("foreign connection grant error = %v, want ErrConnectionNotFound", err)
	}
	if err := GrantConnection(t.Context(), db, org.ID, team.ID, connection.ID, &user.ID); err != nil {
		t.Fatalf("grant required connection: %v", err)
	}

	catalog := model.AgentCatalog{
		ID:                  uuid.New(),
		Slug:                "github-required-" + uuid.NewString()[:8],
		Name:                "GitHub Agent",
		RequiredConnections: pq.StringArray{"github-app"},
		Status:              model.AgentCatalogStatusActive,
	}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatalf("seed catalog agent: %v", err)
	}
	agent := model.Agent{
		ID:             uuid.New(),
		OrgID:          &org.ID,
		TeamID:         team.ID,
		AgentCatalogID: &catalog.ID,
		Name:           "Installed GitHub Agent",
		Status:         "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("seed installed agent: %v", err)
	}

	if err := RevokeConnection(t.Context(), db, org.ID, team.ID, connection.ID); !errors.Is(err, ErrConnectionRequired) {
		t.Fatalf("revoke required connection error = %v, want ErrConnectionRequired", err)
	}
	grants, err := ConnectionGrants(t.Context(), db, org.ID, team.ID)
	if err != nil {
		t.Fatalf("list retained grants: %v", err)
	}
	if len(grants) != 1 || grants[0].ConnectionID == nil || *grants[0].ConnectionID != connection.ID {
		t.Fatalf("required grant was not retained: %#v", grants)
	}

	if err := db.Model(&agent).Update("status", "archived").Error; err != nil {
		t.Fatalf("archive installed agent: %v", err)
	}
	if err := RevokeConnection(t.Context(), db, org.ID, team.ID, connection.ID); err != nil {
		t.Fatalf("revoke after agent archive: %v", err)
	}
	grants, err = ConnectionGrants(t.Context(), db, org.ID, team.ID)
	if err != nil {
		t.Fatalf("list grants after revoke: %v", err)
	}
	if len(grants) != 0 {
		t.Fatalf("grants after revoke = %#v, want none", grants)
	}
}
