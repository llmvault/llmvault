package tasks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

func TestAgentTriggerDispatchHandler_GitHubCheckSuiteMissingPullRequestSkips(t *testing.T) {
	handler := &AgentTriggerDispatchHandler{}
	payload := AgentTriggerDispatchPayload{
		Provider:    "github-app",
		EventType:   "check_suite",
		EventAction: "completed",
	}

	skip, reason, err := handler.shouldSkipTriggerDelivery(t.Context(), payload, map[string]any{
		"check_suite": map[string]any{
			"pull_requests": []any{},
		},
	})
	if err != nil {
		t.Fatalf("filter check suite: %v", err)
	}
	if !skip || reason != "check_suite has no pull request" {
		t.Fatalf("skip=%v reason=%q, want missing PR skip", skip, reason)
	}
}

func TestAgentTriggerDispatchHandler_GitHubCheckSuiteMissingDependenciesErrors(t *testing.T) {
	handler := &AgentTriggerDispatchHandler{}
	_, _, err := handler.shouldSkipTriggerDelivery(t.Context(), AgentTriggerDispatchPayload{
		Provider:    "github-app",
		EventType:   "check_suite",
		EventAction: "completed",
	}, githubCheckSuitePayload())
	if err == nil {
		t.Fatal("expected missing dependency error")
	}
}

func TestAgentTriggerDispatchHandler_NonCheckSuiteEventsPassThrough(t *testing.T) {
	handler := &AgentTriggerDispatchHandler{}
	skip, reason, err := handler.shouldSkipTriggerDelivery(t.Context(), AgentTriggerDispatchPayload{
		Provider:    "github-app",
		EventType:   "issue_comment",
		EventAction: "created",
	}, map[string]any{})
	if err != nil {
		t.Fatalf("filter non-check-suite event: %v", err)
	}
	if skip || reason != "" {
		t.Fatalf("skip=%v reason=%q, want passthrough", skip, reason)
	}
}

func TestAgentTriggerDispatchHandler_GitHubCheckSuiteUsesPullRequestAuthor(t *testing.T) {
	db := openTasksMemoryTestDB(t)
	orgID := uuid.New()
	connID := uuid.New()
	if err := db.Create(&model.Org{ID: orgID, Name: "check-suite-filter-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	user := model.User{ID: uuid.New(), Email: "check-suite-filter-" + uuid.NewString()[:8] + "@test.com"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	integration := model.Integration{
		ID:          uuid.New(),
		UniqueKey:   "github-app-check-suite-filter-" + uuid.NewString()[:8],
		Provider:    "github-app",
		DisplayName: "GitHub",
	}
	if err := db.Create(&integration).Error; err != nil {
		t.Fatalf("create integration: %v", err)
	}
	if err := db.Create(&model.Connection{
		ID:                connID,
		OrgID:             orgID,
		UserID:            user.ID,
		IntegrationID:     integration.ID,
		NangoConnectionID: "github-nango-conn",
	}).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}

	var providerKey, connectionID string
	nangoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerKey = r.Header.Get("Provider-Config-Key")
		connectionID = r.Header.Get("Connection-Id")
		if r.URL.Path != "/proxy/repos/usehivy/hivy/pulls/42" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user": map[string]any{"login": "usehivy[bot]"},
		})
	}))
	t.Cleanup(nangoSrv.Close)

	handler := &AgentTriggerDispatchHandler{
		db:          db,
		nangoClient: nango.NewClient(nangoSrv.URL, "nango-secret"),
	}
	skip, reason, err := handler.shouldSkipTriggerDelivery(t.Context(), AgentTriggerDispatchPayload{
		Provider:     "github-app",
		EventType:    "check_suite",
		EventAction:  "completed",
		OrgID:        orgID,
		ConnectionID: connID,
	}, githubCheckSuitePayload())
	if err != nil {
		t.Fatalf("filter check suite: %v", err)
	}
	if skip || reason != "" {
		t.Fatalf("skip=%v reason=%q, want Hivy-authored delivery", skip, reason)
	}
	if providerKey != integration.UniqueKey || connectionID != "github-nango-conn" {
		t.Fatalf("nango headers provider=%q connection=%q", providerKey, connectionID)
	}
}

func TestAgentTriggerDispatchHandler_GitHubCheckSuiteSkipsNonHivyPullRequestAuthor(t *testing.T) {
	db := openTasksMemoryTestDB(t)
	orgID := uuid.New()
	connID := uuid.New()
	if err := db.Create(&model.Org{ID: orgID, Name: "check-suite-filter-skip-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	user := model.User{ID: uuid.New(), Email: "check-suite-filter-skip-" + uuid.NewString()[:8] + "@test.com"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	integration := model.Integration{
		ID:          uuid.New(),
		UniqueKey:   "github-app-check-suite-skip-" + uuid.NewString()[:8],
		Provider:    "github-app",
		DisplayName: "GitHub",
	}
	if err := db.Create(&integration).Error; err != nil {
		t.Fatalf("create integration: %v", err)
	}
	if err := db.Create(&model.Connection{
		ID:                connID,
		OrgID:             orgID,
		UserID:            user.ID,
		IntegrationID:     integration.ID,
		NangoConnectionID: "github-nango-conn",
	}).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}

	nangoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user": map[string]any{"login": "octocat"},
		})
	}))
	t.Cleanup(nangoSrv.Close)

	handler := &AgentTriggerDispatchHandler{
		db:          db,
		nangoClient: nango.NewClient(nangoSrv.URL, "nango-secret"),
	}
	skip, reason, err := handler.shouldSkipTriggerDelivery(t.Context(), AgentTriggerDispatchPayload{
		Provider:     "github-app",
		EventType:    "check_suite",
		EventAction:  "completed",
		OrgID:        orgID,
		ConnectionID: connID,
	}, githubCheckSuitePayload())
	if err != nil {
		t.Fatalf("filter check suite: %v", err)
	}
	if !skip || reason != "check_suite pull request was not created by Hivy" {
		t.Fatalf("skip=%v reason=%q, want non-Hivy author skip", skip, reason)
	}
}

func githubCheckSuitePayload() map[string]any {
	return map[string]any{
		"repository": map[string]any{
			"name":  "hivy",
			"owner": map[string]any{"login": "usehivy"},
		},
		"check_suite": map[string]any{
			"pull_requests": []any{map[string]any{"number": float64(42)}},
			"app":           map[string]any{"name": "GitHub Actions"},
		},
	}
}
