package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func TestAdvanceOnboarding_EnforcesOrderAndAdvancesOptionalStep(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:onboarding-handler?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	for _, statement := range []string{
		"CREATE TABLE orgs (id text PRIMARY KEY, onboarding_step text NOT NULL, updated_at datetime)",
		"CREATE TABLE org_memberships (id text PRIMARY KEY, user_id text NOT NULL, org_id text NOT NULL, role text NOT NULL, deactivated_at datetime)",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create test schema: %v", err)
		}
	}
	org := model.Org{ID: uuid.New(), OnboardingStep: model.OnboardingStepTeam}
	user := model.User{ID: uuid.New()}
	if err := db.Exec("INSERT INTO orgs (id, onboarding_step) VALUES (?, ?)", org.ID, org.OnboardingStep).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := db.Exec("INSERT INTO org_memberships (id, user_id, org_id, role) VALUES (?, ?, ?, ?)", uuid.New(), user.ID, org.ID, "owner").Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}

	handler := NewOrgHandler(db, nil)
	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPatch, "/v1/orgs/current/onboarding", bytes.NewBufferString(`{"step":"welcome"}`))
		req = middleware.WithOrg(req, &org)
		req = middleware.WithUser(req, &user)
		recorder := httptest.NewRecorder()
		handler.AdvanceOnboarding(recorder, req)
		return recorder
	}

	if recorder := request(); recorder.Code != http.StatusBadRequest {
		t.Fatalf("skip mandatory team status = %d, want 400: %s", recorder.Code, recorder.Body.String())
	}
	if err := db.Model(&model.Org{}).Where("id = ?", org.ID).Update("onboarding_step", model.OnboardingStepConnections).Error; err != nil {
		t.Fatalf("seed connections step: %v", err)
	}
	if recorder := request(); recorder.Code != http.StatusOK {
		t.Fatalf("advance optional step status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
}
