package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestIntegration_SessionsAddParticipants_BulkSharesSession(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Share this with the team")

	share := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/participants", fx, fx.owner, map[string]any{
		"user_ids": []string{
			fx.member.ID.String(),
			fx.bystander.ID.String(),
			fx.member.ID.String(),
		},
	})
	if share.Code != http.StatusOK {
		t.Fatalf("share status=%d body=%s", share.Code, share.Body.String())
	}
	var out struct {
		Session      sessionOut `json:"session"`
		Participants []struct {
			UserID string `json:"user_id"`
			Role   string `json:"role"`
		} `json:"participants"`
	}
	if err := json.Unmarshal(share.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode share response: %v\n%s", err, share.Body.String())
	}
	if out.Session.ParticipantCount != 3 || len(out.Participants) != 3 {
		t.Fatalf("participants=%+v session=%+v, want owner plus two shared users", out.Participants, out.Session)
	}
	roles := map[string]string{}
	for _, participant := range out.Participants {
		roles[participant.UserID] = participant.Role
	}
	if roles[fx.member.ID.String()] != "collaborator" || roles[fx.bystander.ID.String()] != "collaborator" {
		t.Fatalf("participant roles=%+v", roles)
	}
}
