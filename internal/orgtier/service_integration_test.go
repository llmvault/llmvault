package orgtier

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestConcurrentSessionCreationReservesOnlyTheFinalTierOneSlot(t *testing.T) {
	db := connectTierTestDB(t)
	org, agent := createTierTestOrgAndAgent(t, db, Tier1)
	createTierTestSandbox(t, db, org.ID, agent.ID, "running")

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results <- WithSessionCreate(context.Background(), db, org.ID, "nano", func() error {
				sb := model.Sandbox{
					ID: uuid.New(), OrgID: &org.ID, AgentID: &agent.ID,
					ProviderID: "test", ExternalID: fmt.Sprintf("concurrent-%d-%s", index, uuid.NewString()),
					RuntimeURL: "http://runtime.test", EncryptedRuntimeSecret: []byte("test"), Status: "running",
				}
				return db.WithContext(context.Background()).Create(&sb).Error
			})
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	var succeeded, limited int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrConcurrentSessions):
			limited++
		default:
			t.Fatalf("unexpected create result: %v", err)
		}
	}
	if succeeded != 1 || limited != 1 {
		t.Fatalf("succeeded=%d limited=%d, want 1 and 1", succeeded, limited)
	}
}

func TestPendingSessionCreateReservationBlocksAStoppedSandboxWake(t *testing.T) {
	db := connectTierTestDB(t)
	org, agent := createTierTestOrgAndAgent(t, db, Tier1)
	createTierTestSandbox(t, db, org.ID, agent.ID, "running")
	stopped := createTierTestSandbox(t, db, org.ID, agent.ID, "stopped")
	pending := model.OrgSessionCapacityReservation{
		ID: uuid.New(), OrgID: org.ID, ExpiresAt: time.Now().Add(time.Minute),
	}
	if err := db.Create(&pending).Error; err != nil {
		t.Fatalf("create pending reservation: %v", err)
	}

	_, err := ReserveSessionWake(t.Context(), db, org.ID, stopped.ID)
	if !errors.Is(err, ErrConcurrentSessions) {
		t.Fatalf("wake error = %v, want concurrent-session limit", err)
	}
}

func TestKnowledgeStorageReplacementDoesNotDoubleCount(t *testing.T) {
	db := connectTierTestDB(t)
	org, _ := createTierTestOrgAndAgent(t, db, Tier1)
	source := ragmodel.RAGSource{
		ID: uuid.New(), OrgIDValue: org.ID, KindValue: ragmodel.RAGSourceKindFileUpload,
		Name: "source", Status: ragmodel.RAGSourceStatusActive, Enabled: true, ConfigValue: model.JSON{},
	}
	if err := db.Create(&source).Error; err != nil {
		t.Fatalf("create rag source: %v", err)
	}

	if _, err := ReserveDocumentStorage(t.Context(), db, org.ID, source.ID, []DocumentStorage{{DocumentID: "doc-1", Bytes: 400}}); err != nil {
		t.Fatalf("record first size: %v", err)
	}
	if _, err := ReserveDocumentStorage(t.Context(), db, org.ID, source.ID, []DocumentStorage{{DocumentID: "doc-1", Bytes: 600}}); err != nil {
		t.Fatalf("replace size: %v", err)
	}
	used, err := KnowledgeStorageUsed(t.Context(), db, org.ID)
	if err != nil {
		t.Fatalf("knowledge storage used: %v", err)
	}
	if used != 600 {
		t.Fatalf("used = %d, want 600", used)
	}
}

func TestCompletedDepositsPromoteWithoutDowngrading(t *testing.T) {
	db := connectTierTestDB(t)
	org, _ := createTierTestOrgAndAgent(t, db, Tier1)
	purchase := model.CreditPurchase{
		ID: uuid.New(), OrgID: org.ID, PackID: "usd-100", IdempotencyKey: uuid.NewString(),
		Provider: "test", ProviderRef: uuid.NewString(), Status: model.CreditPurchaseCredited,
		Currency: "USD", SubtotalMinor: 10_000, FeeBasisPoints: 1_200, FeeMinor: 1_200,
		TotalMinor: 11_200, Credits: 100_000,
	}
	if err := db.Create(&purchase).Error; err != nil {
		t.Fatalf("create credited purchase: %v", err)
	}
	if err := PromoteForCompletedDeposits(t.Context(), db, org.ID); err != nil {
		t.Fatalf("promote org: %v", err)
	}
	if err := db.Model(&purchase).Update("status", model.CreditPurchaseRefunded).Error; err != nil {
		t.Fatalf("refund purchase: %v", err)
	}
	if err := PromoteForCompletedDeposits(t.Context(), db, org.ID); err != nil {
		t.Fatalf("recalculate org after refund: %v", err)
	}
	var stored model.Org
	if err := db.First(&stored, "id = ?", org.ID).Error; err != nil {
		t.Fatalf("reload org: %v", err)
	}
	if stored.CapacityTier != Tier2 {
		t.Fatalf("capacity tier = %d, want permanent tier 2", stored.CapacityTier)
	}
}

func TestEffectiveSandboxSizeUsesCustomTemplateResources(t *testing.T) {
	db := connectTierTestDB(t)
	org, _ := createTierTestOrgAndAgent(t, db, Tier4)
	tmpl := model.SandboxTemplate{
		ID: uuid.New(), OrgID: &org.ID, Name: "large template", Slug: "large-" + uuid.NewString(),
		Size: "large", Tags: model.JSON{}, Config: model.JSON{}, BuildStatus: "ready",
	}
	if err := db.Create(&tmpl).Error; err != nil {
		t.Fatalf("create sandbox template: %v", err)
	}
	size, err := EffectiveSandboxSize(t.Context(), db, org.ID, "nano", &tmpl.ID)
	if err != nil {
		t.Fatalf("effective sandbox size: %v", err)
	}
	if size != "large" {
		t.Fatalf("effective sandbox size = %q, want large", size)
	}
}

func connectTierTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect Postgres (run `make test-services-up`): %v", err)
	}
	testdb.ApplyMigrations(t, db)
	return db
}

func createTierTestOrgAndAgent(t *testing.T, db *gorm.DB, tier int) (model.Org, model.Agent) {
	t.Helper()
	org := model.Org{ID: uuid.New(), Name: "org-tier-" + uuid.NewString(), Active: true, CapacityTier: tier}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-" + uuid.NewString()}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	agent := model.Agent{
		ID: uuid.New(), OrgID: &org.ID, TeamID: team.ID, Name: "agent-" + uuid.NewString(),
		SandboxImage: model.SandboxImageDefault, SandboxSize: "nano", Model: "test/model", Status: "active",
		Tools: model.JSON{}, McpServers: model.RawJSON("[]"), Skills: model.JSON{}, RuntimeConfig: model.JSON{}, Permissions: model.JSON{}, Resources: model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() { _ = db.Unscoped().Delete(&org).Error })
	return org, agent
}

func createTierTestSandbox(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID, status string) model.Sandbox {
	t.Helper()
	sb := model.Sandbox{
		ID: uuid.New(), OrgID: &orgID, AgentID: &agentID, ProviderID: "test",
		ExternalID: uuid.NewString(), RuntimeURL: "http://runtime.test", EncryptedRuntimeSecret: []byte("test"), Status: status,
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	return sb
}
