package tasks

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

// triggerRuntimeServer records the compiled text of every successful
// PostHTTPMessage. It fails exactly one post — the `failAtPost`-th post it ever
// sees — to simulate a runtime that rejects one trigger mid-batch. Subsequent
// posts (including the asynq retry) succeed. This is order-independent: it fails
// whichever trigger happens to be processed at that position.
type triggerRuntimeServer struct {
	mu         sync.Mutex
	posts      []string
	attempts   int
	failAtPost int
}

func (s *triggerRuntimeServer) recordedPosts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.posts))
	copy(out, s.posts)
	return out
}

func (s *triggerRuntimeServer) handle(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz", "/readyz":
			w.WriteHeader(http.StatusOK)
			return
		case "/gateway/http/messages":
			body, _ := io.ReadAll(r.Body)
			var req employeeruntime.HTTPMessageRequest
			if err := json.Unmarshal(body, &req); err != nil {
				t.Errorf("decode http message: %v", err)
			}
			s.mu.Lock()
			s.attempts++
			shouldFail := s.attempts == s.failAtPost
			if !shouldFail {
				s.posts = append(s.posts, req.Text)
			}
			s.mu.Unlock()
			if shouldFail {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"runtime busy"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(employeeruntime.HTTPMessageResponse{
				SessionID: "rt-" + uuid.NewString(),
				StreamID:  "stream-" + uuid.NewString(),
				TraceID:   "trace-" + uuid.NewString(),
				TurnID:    "turn-" + uuid.NewString(),
			})
			return
		default:
			t.Errorf("unexpected runtime path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// TestEmployeeTriggerDispatch_RetryDeliversOnlyFromFailedTrigger guards P0-14:
// when the dispatch batch fails posting trigger N, asynq retries the whole task.
// Triggers 1..N-1 keep their claims and must NOT be re-posted; only N..end are
// delivered. The agent never re-receives an already-delivered trigger.
func TestEmployeeTriggerDispatch_RetryDeliversOnlyFromFailedTrigger(t *testing.T) {
	db := openTasksMemoryTestDB(t)
	encKey := testTasksEncKey(t)
	cfg := &config.Config{ProxyHost: "proxy.hivy.test"}

	// Fail the second post of the first batch: one trigger is delivered, the next
	// fails, the handler aborts the batch and returns the error.
	rt := &triggerRuntimeServer{failAtPost: 2}
	server := httptest.NewServer(rt.handle(t))
	t.Cleanup(server.Close)

	orgID := uuid.New()
	if err := db.Create(&model.Org{ID: orgID, Name: "trigger-retry-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Employee{ID: uuid.New(), OrgID: &orgID, Name: "Aria", Model: "test-model", Status: "active", IsEmployee: true}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	encryptedSecret, err := encKey.EncryptString("runtime-secret")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	future := time.Now().Add(time.Hour)
	sb := model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  &orgID,
		EmployeeID:             &agent.ID,
		ExternalID:             "trigger-retry-sb",
		RuntimeURL:             server.URL,
		RuntimeURLExpiresAt:    &future,
		EncryptedRuntimeSecret: encryptedSecret,
		Status:                 string(sandbox.StatusRunning),
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	user := model.User{ID: uuid.New(), Email: "trigger-retry-" + uuid.NewString()[:8] + "@test.com"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	integration := model.Integration{
		ID:          uuid.New(),
		UniqueKey:   "github-trigger-retry-" + uuid.NewString()[:8],
		Provider:    "github",
		DisplayName: "GitHub",
	}
	if err := db.Create(&integration).Error; err != nil {
		t.Fatalf("create integration: %v", err)
	}
	connID := uuid.New()
	if err := db.Create(&model.Connection{
		ID:                connID,
		OrgID:             orgID,
		UserID:            user.ID,
		IntegrationID:     integration.ID,
		NangoConnectionID: "trigger-retry-nango-conn",
	}).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}

	// Three webhook triggers, all matching the same event key, each with distinct
	// instructions that appear in the compiled message text so we can assert each
	// trigger is delivered exactly once.
	markers := []string{"TRIGGER-A", "TRIGGER-B", "TRIGGER-C"}
	for _, marker := range markers {
		trig := model.EmployeeTrigger{
			ID:           uuid.New(),
			OrgID:        orgID,
			EmployeeID:   agent.ID,
			TriggerType:  "webhook",
			ConnectionID: &connID,
			TriggerKeys:  pq.StringArray{"issues.opened"},
			Enabled:      true,
			Instructions: "Handle issue for " + marker,
		}
		if err := db.Create(&trig).Error; err != nil {
			t.Fatalf("create trigger %s: %v", marker, err)
		}
	}

	t.Cleanup(func() {
		db.Where("org_id = ?", orgID).Delete(&model.EmployeeTriggerDelivery{})
		db.Where("org_id = ?", orgID).Delete(&model.EmployeeTrigger{})
		db.Where("org_id = ?", orgID).Delete(&model.EmployeeSession{})
		db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Employee{})
		db.Where("id = ?", connID).Delete(&model.Connection{})
		db.Where("id = ?", integration.ID).Delete(&model.Integration{})
		db.Where("id = ?", user.ID).Delete(&model.User{})
		db.Where("id = ?", orgID).Delete(&model.Org{})
	})

	orch := sandbox.NewOrchestrator(db, &employeeUpgradeProvider{endpoint: server.URL}, encKey, cfg)
	handler := &EmployeeTriggerDispatchHandler{
		db:           db,
		orchestrator: orch,
		enqueuer:     &enqueue.MockClient{},
		catalog:      catalog.Global(),
		compileDeps: employeeruntime.CompileDeps{
			DB:         db,
			EncKey:     encKey,
			SigningKey: []byte("test-signing-key-32-bytes-long!!"),
			Cfg:        cfg,
		},
	}

	dispatchPayload := EmployeeTriggerDispatchPayload{
		OrgID:        orgID,
		ConnectionID: connID,
		Provider:     "github",
		EventType:    "issues",
		EventAction:  "opened",
		DeliveryID:   "gh-delivery-1",
		PayloadJSON:  []byte(`{"issue":{"number":7,"title":"Bug"},"repository":{"full_name":"usehivy/hivy","name":"hivy","owner":{"login":"usehivy"}}}`),
	}
	task, _, err := NewEmployeeTriggerDispatchTask(dispatchPayload)
	if err != nil {
		t.Fatalf("build dispatch task: %v", err)
	}

	// First pass: one trigger is delivered, the next fails its post, and the
	// handler returns the error so asynq would retry the whole task.
	if err := handler.Handle(t.Context(), task); err == nil {
		t.Fatalf("first dispatch should fail because a trigger post failed")
	}

	firstPosts := rt.recordedPosts()
	if len(firstPosts) != 1 {
		t.Fatalf("first pass should have delivered exactly one trigger before aborting, got %v", firstPosts)
	}

	// Retry: the already-delivered trigger is claimed (skipped, not re-posted);
	// the failed trigger's claim was released so it re-delivers; the remaining
	// trigger delivers. asynq retries the identical task.
	if err := handler.Handle(t.Context(), task); err != nil {
		t.Fatalf("retry dispatch should succeed: %v", err)
	}

	// Each trigger is delivered exactly once across the failed batch + retry: the
	// first-pass success is not re-posted (claim held), the failed one re-delivers
	// (claim released), and the remaining one delivers.
	posts := rt.recordedPosts()
	if len(posts) != len(markers) {
		t.Fatalf("expected %d total successful posts, got %d (posts=%v)", len(markers), len(posts), posts)
	}
	for _, marker := range markers {
		if got := countContaining(posts, marker); got != 1 {
			t.Fatalf("%s must be delivered exactly once, got %d (posts=%v)", marker, got, posts)
		}
	}

	// Exactly one delivery row per trigger for this delivery id.
	var deliveryRows int64
	db.Model(&model.EmployeeTriggerDelivery{}).
		Where("org_id = ? AND delivery_id = ?", orgID, "gh-delivery-1").
		Count(&deliveryRows)
	if deliveryRows != int64(len(markers)) {
		t.Fatalf("expected %d delivery rows (one per trigger), got %d", len(markers), deliveryRows)
	}
}

func countContaining(items []string, substr string) int {
	n := 0
	for _, item := range items {
		if strings.Contains(item, substr) {
			n++
		}
	}
	return n
}
