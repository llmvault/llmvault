package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *orgUpdateHarness) createUser(t *testing.T) model.User {
	t.Helper()
	user := model.User{Email: "org-create-" + uuid.New().String()[:8] + "@test.com", Name: "Test"}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("id = ?", user.ID).Delete(&model.User{})
	})
	return user
}

func (h *orgUpdateHarness) doCreate(t *testing.T, userID uuid.UUID, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf := new(bytes.Buffer)
	if body != nil {
		_ = json.NewEncoder(buf).Encode(body)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/orgs", buf)
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{UserID: userID.String()})

	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func TestOrgCreate_EnqueuesHivyAgentProvision(t *testing.T) {
	h := newOrgUpdateHarness(t)
	user := h.createUser(t)

	rr := h.doCreate(t, user.ID, map[string]any{"name": "Provisioned Org"})

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d body=%s, want 201", rr.Code, rr.Body.String())
	}
	orgID := assertOrgProvisionTaskEnqueued(t, h.enqueuer)
	cleanupCreatedOrg(t, h, orgID)
}

func cleanupCreatedOrg(t *testing.T, h *orgUpdateHarness, orgID uuid.UUID) {
	t.Helper()
	if orgID == uuid.Nil {
		return
	}
	t.Cleanup(func() {
		h.db.Where("org_id = ?", orgID).Delete(&model.Channel{})
		h.db.Where("org_id = ?", orgID).Delete(&model.Agent{})
		h.db.Where("org_id = ?", orgID).Delete(&model.OrgMembership{})
		h.db.Where("id = ?", orgID).Delete(&model.Org{})
	})
}
