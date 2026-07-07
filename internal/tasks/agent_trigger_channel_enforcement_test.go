package tasks

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// A trigger whose configured channel does not have the agent assigned is
// rejected at session creation (hard enforcement).
func TestFindOrCreateTriggerSessionRejectsUnassignedChannel(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	// A standard channel WITHOUT a channel_agents row for the agent.
	channel := model.Channel{
		OrgID:          org.ID,
		Name:           "unassigned-" + uuid.NewString()[:8],
		Kind:           "standard",
		Visibility:     "public",
		DefaultAgentID: agent.ID,
		Origin:         "native",
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	trigger := seedTriggerForSession(t, db, org.ID, agent.ID, &channel.ID)
	handler := &AgentTriggerDispatchHandler{db: db}

	_, err := handler.findOrCreateTriggerSession(context.Background(), &agent, trigger, "github/acme/repo/issues/1")
	if err == nil || !strings.Contains(err.Error(), "not assigned to this channel") {
		t.Fatalf("err = %v, want not-assigned rejection", err)
	}
}

// The system channel is exempt: a trigger with no configured channel runs any
// org agent there even without an explicit assignment.
func TestFindOrCreateTriggerSessionSystemChannelExempt(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	trigger := seedTriggerForSession(t, db, org.ID, agent.ID, nil)
	handler := &AgentTriggerDispatchHandler{db: db}

	session, err := handler.findOrCreateTriggerSession(context.Background(), &agent, trigger, "github/acme/repo/issues/2")
	if err != nil {
		t.Fatalf("system-channel trigger session: %v", err)
	}
	var channel model.Channel
	if err := db.First(&channel, "id = ?", session.ChannelID).Error; err != nil {
		t.Fatalf("load channel: %v", err)
	}
	if channel.Kind != "system" {
		t.Fatalf("channel kind = %s, want system", channel.Kind)
	}
}
