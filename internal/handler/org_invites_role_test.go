package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/email"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// noopInviteSender satisfies email.Sender without sending anything; invite-role
// validation returns before any email is dispatched.
type noopInviteSender struct{}

func (noopInviteSender) Send(context.Context, email.Message) error                 { return nil }
func (noopInviteSender) SendTemplate(context.Context, email.TemplateMessage) error { return nil }

// TestCreateInvite_RejectsViewerRole asserts the removed "viewer" org role can no
// longer be invited: the request is rejected with 400 before any invite row is
// created. Only "admin" and "member" are valid invite roles.
func TestCreateInvite_RejectsViewerRole(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	inviter := createTestUser(t, db, "inviter-"+org.ID.String()[:8]+"@example.com")

	h := handler.NewOrgInviteHandler(db, noopInviteSender{}, "https://app.test")
	r := chi.NewRouter()
	r.Post("/v1/orgs/current/invites", h.Create)

	do := func(role string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"email": "invitee@example.com", "role": role})
		req := httptest.NewRequest(http.MethodPost, "/v1/orgs/current/invites", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = middleware.WithOrg(req, &org)
		req = middleware.WithAuthClaims(req, &auth.AuthClaims{UserID: inviter.ID.String(), OrgID: org.ID.String()})
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	if rr := do("viewer"); rr.Code != http.StatusBadRequest {
		t.Fatalf("invite with role=viewer: got %d, want 400; body %s", rr.Code, rr.Body.String())
	}

	// Cleanup any invite rows the positive case creates.
	t.Cleanup(func() { db.Where("org_id = ?", org.ID).Delete(&model.OrgInvite{}) })
	if rr := do("member"); rr.Code != http.StatusCreated {
		t.Fatalf("invite with role=member: got %d, want 201; body %s", rr.Code, rr.Body.String())
	}
}
