package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *sessionHarness) doJSON(t *testing.T, method, path string, fx sessionFixture, user model.User, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", fx.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: user.ID.String(),
		OrgID:  fx.org.ID.String(),
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *sessionHarness) createSession(t *testing.T, fx sessionFixture, user model.User, text string) sessionMutationOut {
	t.Helper()
	rr := h.doJSON(t, http.MethodPost, "/v1/sessions", fx, user, map[string]any{
		"channel_id": fx.channel.ID.String(),
		"text":       text,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create session status=%d body=%s", rr.Code, rr.Body.String())
	}
	return decodeSessionMutation(t, rr)
}

// markSessionIdle clears any in-flight turn on a session so it can be archived.
// createSession posts an initial message, which starts a turn (agent_turn_status
// = active); tests that exercise archive permissions/visibility flip the session
// back to idle to simulate the turn completing.
func (h *sessionHarness) markSessionIdle(t *testing.T, id string) {
	t.Helper()
	if err := h.db.Model(&model.Session{}).
		Where("id = ?", id).
		Update("agent_turn_status", model.SessionAgentTurnIdle).Error; err != nil {
		t.Fatalf("mark session idle: %v", err)
	}
}

func decodeSessionMutation(t *testing.T, rr *httptest.ResponseRecorder) sessionMutationOut {
	t.Helper()
	var out sessionMutationOut
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode session mutation: %v\n%s", err, rr.Body.String())
	}
	return out
}

func cleanupSessionFixture(db *gorm.DB, orgID uuid.UUID, userIDs ...uuid.UUID) {
	var sessions []model.Session
	_ = db.Where("org_id = ?", orgID).Find(&sessions).Error
	sessionIDs := make([]uuid.UUID, 0, len(sessions))
	for _, session := range sessions {
		sessionIDs = append(sessionIDs, session.ID)
	}
	if len(sessionIDs) > 0 {
		db.Where("session_id IN ?", sessionIDs).Delete(&model.SessionMessageQueue{})
		db.Where("session_id IN ?", sessionIDs).Delete(&model.SessionEvent{})
		db.Where("session_id IN ?", sessionIDs).Delete(&model.SessionParticipant{})
	}
	db.Where("org_id = ?", orgID).Delete(&model.Session{})
	db.Where("org_id = ?", orgID).Delete(&model.Token{})
	db.Where("org_id = ?", orgID).Delete(&model.Credential{})
	db.Where("org_id = ?", orgID).Delete(&model.ChannelAgent{})
	db.Where("org_id = ?", orgID).Delete(&model.Channel{})
	db.Where("org_id = ?", orgID).Delete(&model.Agent{})
	db.Where("org_id = ?", orgID).Delete(&model.Sandbox{})
	db.Where("org_id = ?", orgID).Delete(&model.OrgMembership{})
	if len(userIDs) > 0 {
		db.Where("id IN ?", userIDs).Delete(&model.User{})
	}
	db.Where("id = ?", orgID).Delete(&model.Org{})
}
