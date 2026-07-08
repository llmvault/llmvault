package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type orgMemberHarness struct {
	db     *gorm.DB
	router *chi.Mux
}

func newOrgMemberHarness(t *testing.T) *orgMemberHarness {
	t.Helper()
	db := connectTestDB(t)
	h := handler.NewOrgMemberHandler(db)

	r := chi.NewRouter()
	r.Patch("/v1/orgs/current/members/{userID}/role", h.PatchRole)
	r.Delete("/v1/orgs/current/members/{userID}", h.Remove)
	r.Post("/v1/orgs/current/transfer-ownership", h.TransferOwnership)
	r.Delete("/v1/orgs/current", h.DeleteOrg)

	return &orgMemberHarness{db: db, router: r}
}

// addMember creates a user and an org membership with the given role, returning
// the user id. All rows are cleaned up when the test finishes.
func (h *orgMemberHarness) addMember(t *testing.T, orgID uuid.UUID, role string) uuid.UUID {
	t.Helper()
	user := createTestUser(t, h.db, "member-"+uuid.NewString()[:8]+"@example.com")
	m := model.OrgMembership{
		ID:     uuid.New(),
		UserID: user.ID,
		OrgID:  orgID,
		Role:   role,
	}
	if err := h.db.Create(&m).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("id = ?", m.ID).Delete(&model.OrgMembership{})
	})
	return user.ID
}

func (h *orgMemberHarness) do(t *testing.T, method, path string, body any, org *model.Org, actingUserID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, org)
	req = middleware.WithUser(req, &model.User{ID: actingUserID})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *orgMemberHarness) roleOf(t *testing.T, orgID, userID uuid.UUID) string {
	t.Helper()
	var m model.OrgMembership
	if err := h.db.Where("org_id = ? AND user_id = ? AND deactivated_at IS NULL", orgID, userID).First(&m).Error; err != nil {
		return ""
	}
	return m.Role
}
