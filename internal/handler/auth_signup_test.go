package handler

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/billing/purchase"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func connectInternalTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func seedSignupUser(t *testing.T, db *gorm.DB) *model.User {
	t.Helper()
	user := &model.User{
		Email: "signup-" + uuid.NewString() + "@test.usehivy.test",
		Name:  "Signup Test",
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Where("user_id = ?", user.ID).Delete(&model.OrgMembership{})
		db.Unscoped().Delete(user)
	})
	return user
}

func cleanupOrgAndLedger(t *testing.T, db *gorm.DB, orgID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		db.Unscoped().Where("org_id = ?", orgID).Delete(&model.Channel{})
		db.Unscoped().Where("org_id = ?", orgID).Delete(&model.Agent{})
		db.Unscoped().Where("org_id = ?", orgID).Delete(&model.CreditLedgerEntry{})
		db.Unscoped().Where("org_id = ?", orgID).Delete(&model.OrgMembership{})
		db.Unscoped().Where("id = ?", orgID).Delete(&model.Org{})
	})
}

func TestCreateUserDefaultOrg_StartsMandatoryTeamOnboarding(t *testing.T) {
	db := connectInternalTestDB(t)
	user := seedSignupUser(t, db)

	var org model.Org
	err := db.Transaction(func(tx *gorm.DB) error {
		var e error
		org, e = createUserDefaultOrg(context.Background(), tx, nil, user)
		return e
	})
	if err != nil {
		t.Fatalf("createUserDefaultOrg: %v", err)
	}
	cleanupOrgAndLedger(t, db, org.ID)

	if org.OnboardingStep != model.OnboardingStepTeam {
		t.Fatalf("onboarding step = %q, want team", org.OnboardingStep)
	}
	for _, value := range []struct {
		name  string
		model any
	}{
		{name: "teams", model: &model.Team{}},
		{name: "agents", model: &model.Agent{}},
		{name: "channels", model: &model.Channel{}},
	} {
		var count int64
		if err := db.Model(value.model).Where("org_id = ?", org.ID).Count(&count).Error; err != nil {
			t.Fatalf("count %s: %v", value.name, err)
		}
		if count != 0 {
			t.Fatalf("%s count = %d, want 0 before onboarding", value.name, count)
		}
	}
}

func TestCreateUserDefaultOrg_GrantsWelcomeCredits(t *testing.T) {
	db := connectInternalTestDB(t)
	credits := billing.NewCreditsService(db)
	user := seedSignupUser(t, db)

	var org model.Org
	err := db.Transaction(func(tx *gorm.DB) error {
		var e error
		org, e = createUserDefaultOrg(context.Background(), tx, credits, user)
		return e
	})
	if err != nil {
		t.Fatalf("createUserDefaultOrg: %v", err)
	}
	cleanupOrgAndLedger(t, db, org.ID)

	bal, err := credits.Balance(org.ID)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if bal != purchase.WelcomeCredits {
		t.Errorf("balance = %d, want %d", bal, purchase.WelcomeCredits)
	}

	var entries []model.CreditLedgerEntry
	if err := db.Where("org_id = ? AND reason = ?", org.ID, billing.ReasonWelcomeGrant).Find(&entries).Error; err != nil {
		t.Fatalf("query welcome entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want one welcome_grant entry, got %d", len(entries))
	}
	if entries[0].RefType != billing.RefTypeSignup || entries[0].RefID != user.ID.String() {
		t.Errorf("unexpected ref tagging: type=%q id=%q", entries[0].RefType, entries[0].RefID)
	}
}
