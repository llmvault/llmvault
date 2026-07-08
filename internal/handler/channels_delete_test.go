package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func TestIntegration_ChannelDeleteArchivesSessionsAndQueuesMemoryDeletion(t *testing.T) {
	enq := &recordingEnqueuer{}
	h := newChannelHarness(t, handler.WithChannelEnqueuer(enq))
	fx := h.seed(t)
	channelID := createChannelForTest(t, h, fx, fx.owner, "ops-"+uuid.NewString()[:8], "public")
	channelUUID := uuid.MustParse(channelID)

	// An idle session (with a sandbox) and a channel memory.
	session := seedChannelRecentSession(t, h, fx, channelUUID, fx.owner.ID, nil, time.Now())
	sandbox := model.Sandbox{
		OrgID: &fx.org.ID, AgentID: &fx.agent.ID, ProviderID: "docker",
		ExternalID: "chan-del-" + uuid.NewString(), RuntimeURL: "http://x",
		EncryptedRuntimeSecret: []byte("x"), Status: "running",
	}
	if err := h.db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	if err := h.db.Model(&model.Session{}).Where("id = ?", session.ID).
		Update("sandbox_id", sandbox.ID).Error; err != nil {
		t.Fatalf("attach sandbox: %v", err)
	}
	if err := h.db.Create(&model.AgentMemory{
		OrgID: fx.org.ID, ChannelID: &channelUUID, Content: "channel fact",
	}).Error; err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	rr := h.doJSON(t, http.MethodDelete, "/v1/channels/"+channelID, fx, fx.owner, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", rr.Code, rr.Body.String())
	}

	// Channel archived.
	var channel model.Channel
	if err := h.db.First(&channel, "id = ?", channelUUID).Error; err != nil {
		t.Fatalf("load channel: %v", err)
	}
	if channel.ArchivedAt == nil {
		t.Fatal("channel not archived")
	}
	// Session archived + ended.
	var reloaded model.Session
	if err := h.db.First(&reloaded, "id = ?", session.ID).Error; err != nil {
		t.Fatalf("load session: %v", err)
	}
	if reloaded.Status != "archived" || reloaded.EndedAt == nil {
		t.Fatalf("session status=%q ended=%v, want archived/ended", reloaded.Status, reloaded.EndedAt)
	}
	// Enqueued: sandbox teardown + channel memory deletion.
	var sawSandboxDelete, sawMemoryDelete bool
	for _, task := range enq.tasks {
		switch task.Type() {
		case tasks.TypeSandboxDelete:
			sawSandboxDelete = true
		case tasks.TypeChannelMemoriesDelete:
			var p tasks.ChannelMemoriesDeletePayload
			if err := json.Unmarshal(task.Payload(), &p); err != nil {
				t.Fatalf("decode memory delete payload: %v", err)
			}
			if p.ChannelID != channelUUID || p.OrgID != fx.org.ID {
				t.Fatalf("memory delete payload = %+v", p)
			}
			sawMemoryDelete = true
		}
	}
	if !sawSandboxDelete {
		t.Fatal("sandbox teardown was not enqueued")
	}
	if !sawMemoryDelete {
		t.Fatal("channel memory deletion was not enqueued")
	}
}

func TestIntegration_ChannelDeleteRejectsNonIdleSession(t *testing.T) {
	enq := &recordingEnqueuer{}
	h := newChannelHarness(t, handler.WithChannelEnqueuer(enq))
	fx := h.seed(t)
	channelID := createChannelForTest(t, h, fx, fx.owner, "busy-"+uuid.NewString()[:8], "public")
	channelUUID := uuid.MustParse(channelID)

	session := seedChannelRecentSession(t, h, fx, channelUUID, fx.owner.ID, nil, time.Now())
	if err := h.db.Model(&model.Session{}).Where("id = ?", session.ID).
		Update("agent_turn_status", model.SessionAgentTurnActive).Error; err != nil {
		t.Fatalf("set active turn: %v", err)
	}

	rr := h.doJSON(t, http.MethodDelete, "/v1/channels/"+channelID, fx, fx.owner, nil)
	if rr.Code != http.StatusConflict {
		t.Fatalf("delete status=%d, want 409; body=%s", rr.Code, rr.Body.String())
	}
	// Nothing was archived or enqueued.
	var channel model.Channel
	if err := h.db.First(&channel, "id = ?", channelUUID).Error; err != nil {
		t.Fatalf("load channel: %v", err)
	}
	if channel.ArchivedAt != nil {
		t.Fatal("channel was archived despite in-progress turn")
	}
	if len(enq.tasks) != 0 {
		t.Fatalf("enqueued %d tasks on rejected delete", len(enq.tasks))
	}
}

// A team must keep at least one channel: deleting a team's only channel is
// rejected (409), but deleting one when the team has others succeeds.
func TestIntegration_ChannelDeleteKeepsTeamLastChannel(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	team := seedChannelTeam(t, h, fx, "Ops-"+uuid.NewString()[:6])

	makeTeamChannel := func(name string) model.Channel {
		ch := model.Channel{
			OrgID: fx.org.ID, TeamID: &team.ID, Name: name, Kind: "standard",
			Visibility: "public", DefaultAgentID: fx.agent.ID, Origin: "native",
			CreatedBy: &fx.owner.ID,
		}
		if err := h.db.Create(&ch).Error; err != nil {
			t.Fatalf("create team channel %s: %v", name, err)
		}
		if err := h.db.Create(&model.ChannelMember{ChannelID: ch.ID, UserID: fx.owner.ID, Role: "owner"}).Error; err != nil {
			t.Fatalf("create channel member: %v", err)
		}
		return ch
	}

	only := makeTeamChannel("ops-only-" + uuid.NewString()[:6])
	rr := h.doJSON(t, http.MethodDelete, "/v1/channels/"+only.ID.String(), fx, fx.owner, nil)
	if rr.Code != http.StatusConflict {
		t.Fatalf("delete last team channel status=%d, want 409; body=%s", rr.Code, rr.Body.String())
	}

	// With a second channel present, deleting one succeeds.
	_ = makeTeamChannel("ops-two-" + uuid.NewString()[:6])
	rr2 := h.doJSON(t, http.MethodDelete, "/v1/channels/"+only.ID.String(), fx, fx.owner, nil)
	if rr2.Code != http.StatusOK {
		t.Fatalf("delete non-last team channel status=%d, want 200; body=%s", rr2.Code, rr2.Body.String())
	}
}
