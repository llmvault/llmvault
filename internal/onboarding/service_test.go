package onboarding

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestOnboardingRequiresTeamAndConnectionBeforeWelcome(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:onboarding?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.Exec("CREATE TABLE orgs (id text PRIMARY KEY, name text NOT NULL, onboarding_step text NOT NULL, updated_at datetime)").Error; err != nil {
		t.Fatalf("create orgs table: %v", err)
	}
	if err := db.Exec("CREATE TABLE connections (id text PRIMARY KEY, org_id text NOT NULL, revoked_at datetime)").Error; err != nil {
		t.Fatalf("create connections table: %v", err)
	}
	org := model.Org{ID: uuid.New(), Name: "Test", OnboardingStep: model.OnboardingStepTeam}
	if err := db.Exec("INSERT INTO orgs (id, name, onboarding_step) VALUES (?, ?, ?)", org.ID, org.Name, org.OnboardingStep).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}

	service := New(db)
	if err := service.Advance(context.Background(), org.ID, model.OnboardingStepWelcome); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("skip team error = %v, want ErrInvalidTransition", err)
	}
	if err := service.TeamCreated(context.Background(), org.ID); err != nil {
		t.Fatalf("team created: %v", err)
	}
	if err := service.Advance(context.Background(), org.ID, model.OnboardingStepWelcome); !errors.Is(err, ErrConnectionRequired) {
		t.Fatalf("advance without connection error = %v, want ErrConnectionRequired", err)
	}
	if err := db.Exec("INSERT INTO connections (id, org_id) VALUES (?, ?)", uuid.New(), org.ID).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}
	if err := service.Advance(context.Background(), org.ID, model.OnboardingStepWelcome); err != nil {
		t.Fatalf("advance to welcome: %v", err)
	}
	if err := service.Advance(context.Background(), org.ID, model.OnboardingStepComplete); err != nil {
		t.Fatalf("complete onboarding: %v", err)
	}

	if err := db.Raw("SELECT onboarding_step FROM orgs WHERE id = ?", org.ID).Scan(&org.OnboardingStep).Error; err != nil {
		t.Fatalf("reload onboarding step: %v", err)
	}
	if org.OnboardingStep != model.OnboardingStepComplete {
		t.Fatalf("step = %q, want complete", org.OnboardingStep)
	}
}
