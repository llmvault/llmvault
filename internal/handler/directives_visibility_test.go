package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func seedDirectiveRow(t *testing.T, db *gorm.DB, orgID uuid.UUID, channelID *uuid.UUID, content string) uuid.UUID {
	t.Helper()
	directive := model.AgentDirective{
		OrgID:     orgID,
		ChannelID: channelID,
		Content:   content,
		Source:    model.DirectiveSourceUserPinned,
		Active:    true,
	}
	if err := db.Create(&directive).Error; err != nil {
		t.Fatalf("seed directive: %v", err)
	}
	return directive.ID
}

// TestIntegration_DirectivesListVisibility covers L7: a member must not receive
// directives referencing channels they cannot use; global and usable-channel
// directives remain, and owners see everything.
func TestIntegration_DirectivesListVisibility(t *testing.T) {
	h := newMemoryControlHarness(t)
	fx, _ := h.seed(t)
	ch := seedTeamVisibility(t, h.db, fx)

	global := seedDirectiveRow(t, h.db, fx.org.ID, nil, "global rule")
	usable := seedDirectiveRow(t, h.db, fx.org.ID, &ch.usable.ID, "usable channel rule")
	unusable := seedDirectiveRow(t, h.db, fx.org.ID, &ch.unusable.ID, "unusable channel rule")

	decode := func(rr *httptest.ResponseRecorder) map[string]bool {
		if rr.Code != http.StatusOK {
			t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
		}
		var out struct {
			Data []directiveOut `json:"data"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		got := map[string]bool{}
		for _, d := range out.Data {
			got[d.ID] = true
		}
		return got
	}

	memberGot := decode(h.doJSON(t, http.MethodGet, "/v1/directives", fx, fx.member, nil))
	if !memberGot[global.String()] || !memberGot[usable.String()] {
		t.Fatalf("member should see global + usable directives: %v", memberGot)
	}
	if memberGot[unusable.String()] {
		t.Fatalf("LEAK: member saw directive in unusable channel")
	}

	ownerGot := decode(h.doJSON(t, http.MethodGet, "/v1/directives", fx, fx.owner, nil))
	if !ownerGot[global.String()] || !ownerGot[usable.String()] || !ownerGot[unusable.String()] {
		t.Fatalf("owner should see all directives: %v", ownerGot)
	}
}
