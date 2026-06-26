package tasks

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
)

func TestWriteModelUsageCreatesGenerationAndSessionEventIdempotently(t *testing.T) {
	db := connectTestDB(t)
	org := model.Org{Name: "model-usage-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{
		OrgID:           &org.ID,
		Name:            "metering-agent-" + uuid.NewString()[:8],
		SandboxStrategy: "per_session",
		Model:           "deepseek-v4-flash",
		Tools:           model.JSON{},
		McpServers:      model.RawJSON("[]"),
		Skills:          model.JSON{},
		RuntimeConfig:   model.JSON{},
		Permissions:     model.JSON{},
		Resources:       model.JSON{},
		Status:          "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	channel := model.Channel{
		OrgID:          org.ID,
		Name:           "model-usage-" + uuid.NewString()[:8],
		Kind:           "standard",
		Visibility:     "public",
		DefaultAgentID: agent.ID,
		Origin:         "native",
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	sandbox := model.Sandbox{
		OrgID:                  &org.ID,
		AgentID:                &agent.ID,
		ProviderID:             "docker",
		ExternalID:             "model-usage-" + uuid.NewString(),
		RuntimeURL:             "http://runtime.test",
		EncryptedRuntimeSecret: []byte("encrypted"),
		Status:                 "running",
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	session := model.Session{
		OrgID:             org.ID,
		ChannelID:         channel.ID,
		AgentID:           agent.ID,
		SandboxID:         &sandbox.ID,
		Model:             agent.Model,
		ReasoningEffort:   "high",
		Source:            "web",
		SourceResourceKey: uuid.NewString(),
		Status:            "active",
		AgentTurnStatus:   model.SessionAgentTurnIdle,
		IntegrationScopes: model.JSON{},
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	cred := model.Credential{
		Label:        "system-openrouter",
		BaseURL:      "https://openrouter.test/api/v1",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("key"),
		WrappedDEK:   []byte("dek"),
		ProviderID:   "openrouter",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", cred.ID).Delete(&model.Credential{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	now := time.Now().UTC()
	genID := "gen_" + uuid.NewString()
	payload := ModelUsageWritePayload{
		Generation: model.Generation{
			ID:             genID,
			OrgID:          org.ID,
			CredentialID:   cred.ID,
			TokenJTI:       "system:model_usage_test",
			ProviderID:     "openrouter",
			Model:          "gemini-3.5-flash",
			RequestPath:    "/test/model_usage",
			InputTokens:    10,
			OutputTokens:   20,
			Cost:           0.012,
			UpstreamStatus: 200,
			UserID:         uuid.NewString(),
			CreatedAt:      now,
			IsSystem:       true,
		},
		SessionEvent: &model.SessionEvent{
			OrgID:            org.ID,
			SessionID:        session.ID,
			AgentID:          agent.ID,
			SandboxID:        &sandbox.ID,
			RuntimeSessionID: session.ID.String(),
			EventID:          "model_usage:" + genID,
			EventType:        runtimeevents.EventModelUsage,
			Source:           "web",
			Durability:       "durable",
			Payload: model.JSON{
				"generation_id": genID,
				"usage":         model.JSON{"cost": 0.012},
			},
			EventAt: now,
		},
	}

	for i := 0; i < 2; i++ {
		if err := WriteModelUsage(context.Background(), db, payload); err != nil {
			t.Fatalf("write model usage attempt %d: %v", i+1, err)
		}
	}

	var generationCount int64
	if err := db.Model(&model.Generation{}).Where("id = ?", genID).Count(&generationCount).Error; err != nil {
		t.Fatalf("count generations: %v", err)
	}
	if generationCount != 1 {
		t.Fatalf("generation count = %d, want 1", generationCount)
	}
	var eventCount int64
	if err := db.Model(&model.SessionEvent{}).
		Where("session_id = ? AND event_id = ?", session.ID, "model_usage:"+genID).
		Count(&eventCount).Error; err != nil {
		t.Fatalf("count session events: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("event count = %d, want 1", eventCount)
	}
	var generation model.Generation
	if err := db.Where("id = ?", genID).First(&generation).Error; err != nil {
		t.Fatalf("load generation: %v", err)
	}
	if !generation.IsSystem {
		t.Fatalf("generation IsSystem = false, want true")
	}
	if generation.Cost != 0.012 {
		t.Fatalf("generation cost = %v, want 0.012", generation.Cost)
	}
}
