package handler_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

type externalGatewayRuntime struct {
	messages []gateway.RuntimeMessage
}

func (r *externalGatewayRuntime) Send(_ context.Context, message gateway.RuntimeMessage) (*gateway.RuntimeDelivery, error) {
	r.messages = append(r.messages, message)
	return &gateway.RuntimeDelivery{
		SessionID:         "runtime-session-1",
		ResponseStreamID:  "response-stream-1",
		ResponseStreamURL: "/gateway/http/response-streams/response-stream-1",
		TraceID:           "trace-1",
		TurnID:            "turn-1",
	}, nil
}

func TestGatewayExternalRouteCreateAndInboundEnqueuesCallback(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	agent := model.Agent{OrgID: &org.ID, Model: "deepseek-v4-flash", Status: "active"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	encKey := testGatewayEncKey(t)
	runtimeSecret, err := encKey.EncryptString("runtime-secret")
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sandbox := model.Sandbox{
		OrgID:                  &org.ID,
		AgentID:                &agent.ID,
		ExternalID:             "external-gateway-sandbox",
		RuntimeURL:             "https://runtime.example.test",
		EncryptedRuntimeSecret: runtimeSecret,
		Status:                 "running",
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	runtime := &externalGatewayRuntime{}
	service := gateway.NewService(db, runtime, encKey, gateway.NewExternalAdapter())
	enq := &enqueue.MockClient{}
	h := handler.NewGatewayExternalHandler(db, service, encKey, enq, "https://api.usehivy.test")

	router := chi.NewRouter()
	router.Post("/v1/agents/{id}/gateway-routes", h.CreateRoute)
	router.Post("/incoming/gateways/external/{routeID}", h.HandleInbound)

	createBody := []byte(`{"name":"WhatsApp support","provider":"whatsapp","callback_url":"https://wrapper.example.test/hivy/callback"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/"+agent.ID.String()+"/gateway-routes", bytes.NewReader(createBody))
	req = middleware.WithOrg(req, &org)
	createResp := httptest.NewRecorder()
	router.ServeHTTP(createResp, req)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		ID        string `json:"id"`
		Secret    string `json:"secret"`
		Inbound   string `json:"inbound_url"`
		Provider  string `json:"provider"`
		Callback  string `json:"callback_url"`
		SecretPre string `json:"secret_prefix"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Secret == "" || created.Inbound == "" || created.Provider != "whatsapp" || created.Callback == "" || created.SecretPre == "" {
		t.Fatalf("create response missing fields: %#v", created)
	}

	inboundBody := []byte(`{"message_id":"wa-msg-1","thread_id":"wa-group-1","channel_id":"wa-group-1","sender":{"id":"user-1","name":"Ada"},"text":"Can you check sales?"}`)
	inboundReq := httptest.NewRequest(http.MethodPost, "/incoming/gateways/external/"+created.ID, bytes.NewReader(inboundBody))
	inboundReq.Header.Set("Authorization", "Bearer "+created.Secret)
	inboundReq.Header.Set("Content-Type", "application/json")
	inboundResp := httptest.NewRecorder()
	router.ServeHTTP(inboundResp, inboundReq)
	if inboundResp.Code != http.StatusAccepted {
		t.Fatalf("inbound status = %d: %s", inboundResp.Code, inboundResp.Body.String())
	}
	if len(runtime.messages) != 1 {
		t.Fatalf("runtime messages = %d, want 1", len(runtime.messages))
	}
	if runtime.messages[0].Text != "Can you check sales?" || runtime.messages[0].GatewayProvider != "whatsapp" {
		t.Fatalf("runtime message = %#v", runtime.messages[0])
	}
	enqueued := enq.Tasks()
	if len(enqueued) != 1 || enqueued[0].TypeName != tasks.TypeGatewayExternalCallback {
		t.Fatalf("enqueued = %#v", enqueued)
	}
	var payload tasks.GatewayExternalCallbackPayload
	if err := json.Unmarshal(enqueued[0].Payload, &payload); err != nil {
		t.Fatalf("decode callback payload: %v", err)
	}
	if payload.CallbackURL != "https://wrapper.example.test/hivy/callback" || payload.RouteSecret != created.Secret || payload.StreamURL != "https://runtime.example.test/gateway/http/response-streams/response-stream-1" {
		t.Fatalf("callback payload = %#v", payload)
	}
}

func testGatewayEncKey(t *testing.T) *crypto.SymmetricKey {
	t.Helper()
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	encKey, err := crypto.NewSymmetricKey(key)
	if err != nil {
		t.Fatalf("new symmetric key: %v", err)
	}
	return encKey
}

func TestGatewayExternalInboundRejectsWrongSecret(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	agentID := uuid.New()
	agent := model.Agent{ID: agentID, OrgID: &org.ID, Model: "deepseek-v4-flash", Status: "active"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	encKey := testGatewayEncKey(t)
	secret, hash, prefix, err := gateway.GenerateExternalGatewaySecret()
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}
	encrypted, err := encKey.EncryptString(secret)
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	route := model.AgentGatewayRoute{
		OrgID:    org.ID,
		AgentID:  agent.ID,
		Provider: "signal",
		Name:     "Signal",
		Enabled:  true,
		Config: model.JSON{
			"adapter":          gateway.ExternalAdapterName,
			"callback_url":     "https://wrapper.example.test/callback",
			"secret_hash":      hash,
			"secret_prefix":    prefix,
			"encrypted_secret": base64.StdEncoding.EncodeToString(encrypted),
		},
	}
	if err := db.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}
	service := gateway.NewService(db, &externalGatewayRuntime{}, encKey, gateway.NewExternalAdapter())
	h := handler.NewGatewayExternalHandler(db, service, encKey, &enqueue.MockClient{}, "https://api.usehivy.test")
	router := chi.NewRouter()
	router.Post("/incoming/gateways/external/{routeID}", h.HandleInbound)

	req := httptest.NewRequest(http.MethodPost, "/incoming/gateways/external/"+route.ID.String(), bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer wrong")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", resp.Code, resp.Body.String())
	}
}
