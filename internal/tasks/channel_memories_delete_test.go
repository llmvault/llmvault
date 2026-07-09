package tasks

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestChannelMemoriesDeleteHandler(t *testing.T) {
	db := connectTestDB(t)
	ctx := context.Background()

	org := model.Org{Name: "chan-mem-del-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	team := model.Team{OrgID: org.ID, Name: "cmd-team-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	agent := model.Agent{
		OrgID: &org.ID, TeamID: team.ID, Name: "cmd-" + uuid.NewString()[:8], Model: "m",
		Tools: model.JSON{}, McpServers: model.RawJSON("[]"), Skills: model.JSON{},
		RuntimeConfig: model.JSON{}, Permissions: model.JSON{}, Resources: model.JSON{}, Status: "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	newChannel := func() uuid.UUID {
		ch := model.Channel{OrgID: org.ID, TeamID: team.ID, Name: "c-" + uuid.NewString()[:8], Kind: "standard", Visibility: "public", DefaultAgentID: agent.ID, Origin: "native"}
		if err := db.Create(&ch).Error; err != nil {
			t.Fatalf("create channel: %v", err)
		}
		return ch.ID
	}
	orgID := org.ID
	channelID := newChannel()
	otherChannel := newChannel()
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	seed := func(ch *uuid.UUID, content string) {
		if err := db.Create(&model.AgentMemory{OrgID: orgID, ChannelID: ch, Content: content}).Error; err != nil {
			t.Fatalf("seed memory: %v", err)
		}
	}
	seed(&channelID, "deleted channel fact one")
	seed(&channelID, "deleted channel fact two")
	seed(&otherChannel, "other channel fact")
	seed(nil, "global fact")
	t.Cleanup(func() { db.Where("org_id = ?", orgID).Delete(&model.AgentMemory{}) })

	task, _, err := NewChannelMemoriesDeleteTask(ChannelMemoriesDeletePayload{OrgID: orgID, ChannelID: channelID})
	if err != nil {
		t.Fatalf("build task: %v", err)
	}
	if err := NewChannelMemoriesDeleteHandler(db).Handle(ctx, task); err != nil {
		t.Fatalf("handle: %v", err)
	}

	var remaining int64
	db.Model(&model.AgentMemory{}).Where("org_id = ? AND channel_id = ?", orgID, channelID).Count(&remaining)
	if remaining != 0 {
		t.Fatalf("channel memories remaining=%d, want 0", remaining)
	}
	// Other channel + global untouched.
	var others int64
	db.Model(&model.AgentMemory{}).Where("org_id = ?", orgID).Count(&others)
	if others != 2 {
		t.Fatalf("other memories=%d, want 2 (other channel + global)", others)
	}
}
