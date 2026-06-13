package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// A runtime outbox redelivery of the same model-usage event must not create a
// second billable generation row.
func TestRuntimeModelUsageGenerationIsIdempotent(t *testing.T) {
	db := connectAgentSkillSyncTestDB(t)
	org := model.Org{Name: "runtime-usage-idem-" + uuid.NewString(), RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	credential := model.Credential{
		Label: "idem", BaseURL: "https://openrouter.ai/api/v1", AuthScheme: "bearer",
		EncryptedKey: []byte("enc"), WrappedDEK: []byte("dek"), ProviderID: "openrouter",
	}
	if err := db.Create(&credential).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	credentialID := credential.ID
	agent := model.Agent{OrgID: &org.ID, CredentialID: &credentialID, Name: "Aria", Model: "deepseek-v4-flash", IsManaged: true}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sandbox := model.Sandbox{OrgID: &org.ID, AgentID: &agent.ID, ExternalID: "runtime-usage-idem-sandbox", RuntimeURL: "http://localhost:7080", EncryptedRuntimeSecret: []byte("key"), Status: "running"}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	token := model.Token{
		OrgID: org.ID, CredentialID: credential.ID, JTI: uuid.NewString(), ExpiresAt: time.Now().Add(time.Hour),
		Meta: model.JSON{"agent_id": agent.ID.String(), "sandbox_id": sandbox.ID.String(), "type": "agent_proxy", "user": "u"},
	}
	if err := db.Create(&token).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Generation{})
		db.Where("id = ?", token.ID).Delete(&model.Token{})
		db.Where("id = ?", sandbox.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
		db.Where("id = ?", credential.ID).Delete(&model.Credential{})
	})

	body, err := json.Marshal(runtimeUsagePayload())
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	h := NewAgentOutboundWebhookHandler(db, nil, nil)
	event := func() *agentOutboundEvent {
		return &agentOutboundEvent{EventType: "agent.run.model.usage", Payload: append([]byte(nil), body...), At: time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)}
	}

	if err := h.storeAndMaybeEnqueue(t.Context(), &sandbox, event()); err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	if err := h.storeAndMaybeEnqueue(t.Context(), &sandbox, event()); err != nil {
		t.Fatalf("redelivery ingest should ack, got %v", err)
	}

	var count int64
	db.Model(&model.Generation{}).Where("org_id = ?", org.ID).Count(&count)
	if count != 1 {
		t.Fatalf("generation rows = %d, want 1 (idempotent)", count)
	}
}

// When the generation row cannot be persisted, the webhook must return 5xx so
// the outbox retries, not ack 200 and lose the billing record.
func TestRuntimeModelUsageWebhookReturns5xxOnPersistFailure(t *testing.T) {
	db := connectAgentSkillSyncTestDB(t)
	encKey := outboundWebhookTestSymmetricKey(t)
	org := model.Org{Name: "runtime-usage-5xx-" + uuid.NewString(), RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{OrgID: &org.ID, Name: "Aria", Model: "deepseek-v4-flash", IsManaged: true}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	runtimeSecret := "usage-5xx-secret-" + uuid.NewString()
	encryptedKey, err := encKey.EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	sandbox := model.Sandbox{OrgID: &org.ID, AgentID: &agent.ID, ExternalID: "runtime-usage-5xx-sandbox", RuntimeURL: "http://localhost:7080", EncryptedRuntimeSecret: encryptedKey, Status: "running"}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", sandbox.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	h := NewAgentOutboundWebhookHandler(db, encKey, nil)
	r := chi.NewRouter()
	r.Post("/internal/webhooks/agent/{sandboxID}", h.Handle)

	// No proxy token row exists for this sandbox, so the generation lookup fails
	// (token lookup error) — the webhook must return 5xx, not 200.
	body, err := json.Marshal(runtimeUsagePayload())
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	envelope, err := json.Marshal(agentOutboundEvent{EventType: "agent.run.model.usage", Payload: body, At: time.Now().UTC()})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/internal/webhooks/agent/"+sandbox.ID.String(), bytes.NewReader(envelope))
	req.Header.Set("X-Hivy-Signature", "sha256="+hmacHex(runtimeSecret, envelope))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 so the outbox retries", rec.Code)
	}
}
