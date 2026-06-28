package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

func TestTriggerHandlerListSlackReactionTriggers(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	org, conn := seedNangoSlackConnection(t, db)
	agent := seedSlackReactionAgent(t, db, org.ID)
	channel := seedSlackReactionChannel(t, db, org.ID, conn, agent)
	trigger := seedSlackReactionTrigger(t, db, org.ID, conn, channel, agent, "eyes")
	req := httptest.NewRequest(http.MethodGet, "/v1/triggers", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()

	NewTriggerHandler(db).List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp triggerListResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("triggers=%d want 1", len(resp.Data))
	}
	got := resp.Data[0]
	if got.ID != trigger.ID.String() || got.Provider != slackapp.Provider ||
		got.AgentID != agent.ID.String() || got.AgentName != agent.Name ||
		got.ChannelID != channel.ID.String() || got.ExternalResourceKey != channel.ExternalResourceKey ||
		got.TriggerKey != slackapp.EventReactionAdded || got.TriggerValue != "eyes" {
		t.Fatalf("trigger response=%+v", got)
	}
}

func TestTriggerHandlerUpdateSlackReactionTrigger(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	org, conn := seedNangoSlackConnection(t, db)
	agent := seedSlackReactionAgent(t, db, org.ID)
	channel := seedSlackReactionChannel(t, db, org.ID, conn, agent)
	trigger := seedSlackReactionTrigger(t, db, org.ID, conn, channel, agent, "eyes")
	provisioner := &fakeTriggerProvisioner{}
	body, _ := json.Marshal(map[string]any{
		"provider":               slackapp.Provider,
		"connection_id":          conn.ID.String(),
		"external_resource_key":  "C222",
		"external_resource_name": "launch",
		"agent_id":               agent.ID.String(),
		"trigger_key":            slackapp.EventReactionAdded,
		"trigger_value":          ":rocket:",
		"instructions":           "Reply with the launch plan.",
	})
	req := triggerRouteRequest(http.MethodPatch, "/v1/triggers/"+trigger.ID.String(), trigger.ID.String(), body)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()

	NewTriggerHandler(db, WithTriggerExternalProvisioner(provisioner)).Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(provisioner.calls) != 1 || provisioner.calls[0].ResourceKey != "C222" {
		t.Fatalf("provisioner calls=%+v", provisioner.calls)
	}
	var updated model.AgentTrigger
	if err := db.First(&updated, "id = ?", trigger.ID).Error; err != nil {
		t.Fatalf("load trigger: %v", err)
	}
	if updated.TriggerValue != "rocket" || updated.Instructions != "Reply with the launch plan." {
		t.Fatalf("updated trigger=%+v", updated)
	}
	var updatedChannel model.Channel
	if updated.ChannelID == nil {
		t.Fatalf("updated channel id is nil")
	}
	if err := db.First(&updatedChannel, "id = ?", *updated.ChannelID).Error; err != nil {
		t.Fatalf("load channel: %v", err)
	}
	if updatedChannel.ExternalResourceKey != "C222" || updatedChannel.ExternalResourceName != "launch" {
		t.Fatalf("updated channel=%+v", updatedChannel)
	}
}

func seedSlackReactionTrigger(t *testing.T, db *gorm.DB, orgID uuid.UUID, conn model.Connection, channel model.Channel, agent model.Agent, value string) model.AgentTrigger {
	t.Helper()
	trigger := model.AgentTrigger{
		OrgID:        orgID,
		AgentID:      agent.ID,
		TriggerType:  "webhook",
		ChannelID:    &channel.ID,
		ConnectionID: &conn.ID,
		TriggerKeys:  pq.StringArray{slackapp.EventReactionAdded},
		TriggerKey:   slackapp.EventReactionAdded,
		TriggerValue: value,
		Enabled:      true,
		SourceSlug:   triggerSourceSlug(slackapp.Provider, slackapp.EventReactionAdded, channel.ExternalResourceKey, value),
		Instructions: "Summarize the reacted message.",
	}
	if err := db.Create(&trigger).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	return trigger
}

func triggerRouteRequest(method, target, triggerID string, body []byte) *http.Request {
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", triggerID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}
