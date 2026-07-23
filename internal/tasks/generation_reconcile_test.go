package tasks_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

type fakeDecryptor struct {
	baseURL    string
	providerID string
}

func (f fakeDecryptor) GetDecryptedCredentialByID(_ context.Context, _ string) (*cache.DecryptedCredential, error) {
	return &cache.DecryptedCredential{
		APIKey:     []byte("sk-test-key"),
		BaseURL:    f.baseURL,
		ProviderID: f.providerID,
	}, nil
}

func TestGenerationReconcile_BackfillsZeroUsageRow(t *testing.T) {
	db := connectDB(t)
	orgID := seedTaskTestOrg(t, db)

	fixture, err := os.ReadFile("testdata/openrouter_generation.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var gotPath, gotID, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotID = r.URL.Query().Get("id")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	genID := "gen_" + uuid.NewString()
	orGenID := "gen-test-abc123"
	if err := db.Create(&model.Generation{
		ID:                     genID,
		OrgID:                  orgID,
		CredentialID:           uuid.New(),
		TokenJTI:               uuid.NewString(),
		ProviderID:             "openrouter",
		Model:                  "deepseek-v4-flash",
		IsSystem:               true,
		InputTokens:            0,
		OutputTokens:           0,
		OpenRouterGenerationID: &orGenID,
		CreatedAt:              time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("insert zero-usage generation: %v", err)
	}

	handler := tasks.NewGenerationReconcileHandler(db, fakeDecryptor{baseURL: srv.URL, providerID: "openrouter"})
	if err := handler.Handle(context.Background(), asynq.NewTask(tasks.TypeGenerationReconcile, nil)); err != nil {
		t.Fatalf("reconcile handler: %v", err)
	}

	if gotPath != "/generation" {
		t.Errorf("request path = %q, want /generation", gotPath)
	}
	if gotID != orGenID {
		t.Errorf("query id = %q, want %q", gotID, orGenID)
	}
	if gotAuth != "Bearer sk-test-key" {
		t.Errorf("authorization = %q, want Bearer sk-test-key", gotAuth)
	}

	var g model.Generation
	if err := db.Where("id = ?", genID).First(&g).Error; err != nil {
		t.Fatalf("load generation: %v", err)
	}
	if g.InputTokens != 1000 {
		t.Errorf("input_tokens = %d, want 1000", g.InputTokens)
	}
	if g.OutputTokens != 500 {
		t.Errorf("output_tokens = %d, want 500", g.OutputTokens)
	}

	if g.Cost != 0.00042 {
		t.Errorf("cost = %.12f, want provider-reported 0.00042", g.Cost)
	}
	if g.BillingCostSource != "provider_reported" {
		t.Errorf("billing_cost_source = %q, want provider_reported", g.BillingCostSource)
	}
	if g.BilledAt != nil {
		t.Errorf("billed_at should remain NULL so the billing batch charges the row")
	}
}

func TestGenerationReconcile_SkipsNonOpenRouterCredential(t *testing.T) {
	db := connectDB(t)
	orgID := seedTaskTestOrg(t, db)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("fetcher should not be called for a non-OpenRouter credential")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	genID := "gen_" + uuid.NewString()
	orGenID := "gen-xyz"
	if err := db.Create(&model.Generation{
		ID:                     genID,
		OrgID:                  orgID,
		CredentialID:           uuid.New(),
		TokenJTI:               uuid.NewString(),
		ProviderID:             "openai",
		Model:                  "gpt-5.4",
		IsSystem:               true,
		OpenRouterGenerationID: &orGenID,
		CreatedAt:              time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("insert generation: %v", err)
	}

	handler := tasks.NewGenerationReconcileHandler(db, fakeDecryptor{baseURL: srv.URL, providerID: "openai"})
	if err := handler.Handle(context.Background(), asynq.NewTask(tasks.TypeGenerationReconcile, nil)); err != nil {
		t.Fatalf("reconcile handler: %v", err)
	}

	var g model.Generation
	if err := db.Where("id = ?", genID).First(&g).Error; err != nil {
		t.Fatalf("load generation: %v", err)
	}
	if g.InputTokens != 0 || g.OutputTokens != 0 {
		t.Errorf("non-OpenRouter row should be untouched, got in=%d out=%d", g.InputTokens, g.OutputTokens)
	}
}
