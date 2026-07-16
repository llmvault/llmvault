package integrations

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestSyncConfiguredProjectsNangoIntegrationsIntoDatabase(t *testing.T) {
	db, err := gorm.Open(postgres.Open(testdb.DatabaseURL()), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	testdb.ApplyMigrations(t, db)
	if err := db.Where("1 = 1").Delete(&model.Connection{}).Error; err != nil {
		t.Fatalf("clear connections: %v", err)
	}
	if err := db.Unscoped().Where("1 = 1").Delete(&model.Integration{}).Error; err != nil {
		t.Fatalf("clear integrations: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Where("1 = 1").Delete(&model.Connection{}).Error
		_ = db.Unscoped().Where("1 = 1").Delete(&model.Integration{}).Error
	})

	fixture := &configuredIntegrationsFixture{integrations: []nango.Integration{
		{UniqueKey: "github-app-code-reviews", Provider: "github-app", DisplayName: "GitHub Code Reviews", Logo: "https://nango.test/github.svg"},
		{UniqueKey: "slack-production", Provider: "slack", DisplayName: "Production Slack", Logo: "https://nango.test/slack.svg"},
	}}
	server := httptest.NewServer(fixture)
	t.Cleanup(server.Close)
	client := nango.NewClient(server.URL, "test-secret")
	if err := client.FetchProviders(t.Context()); err != nil {
		t.Fatalf("fetch providers: %v", err)
	}

	first, err := SyncConfigured(t.Context(), db, client, catalog.Global())
	if err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	if first.Discovered != 2 || first.Created != 2 || first.Updated != 0 || first.Unavailable != 0 {
		t.Fatalf("unexpected initial result: %+v", first)
	}

	var reviews model.Integration
	if err := db.Where("unique_key = ?", "github-app-code-reviews").First(&reviews).Error; err != nil {
		t.Fatalf("load code-review integration: %v", err)
	}
	if reviews.Provider != "github-app-code-reviews" || reviews.BotHandle != "usehivy-reviews" {
		t.Fatalf("code-review projection = provider %q bot %q", reviews.Provider, reviews.BotHandle)
	}
	if reviews.NangoConfig["auth_mode"] != "APP" {
		t.Fatalf("code-review auth mode = %v, want APP", reviews.NangoConfig["auth_mode"])
	}

	fixture.replace([]nango.Integration{
		{UniqueKey: "github-app-code-reviews", Provider: "github-app", DisplayName: "GitHub Reviews", Logo: "https://nango.test/github.svg"},
	})
	second, err := SyncConfigured(t.Context(), db, client, catalog.Global())
	if err != nil {
		t.Fatalf("reconcile changed integrations: %v", err)
	}
	if second.Discovered != 1 || second.Updated != 1 || second.Unavailable != 1 {
		t.Fatalf("unexpected reconciliation result: %+v", second)
	}
	if err := db.Where("unique_key = ?", "github-app-code-reviews").First(&reviews).Error; err != nil {
		t.Fatalf("reload code-review integration: %v", err)
	}
	if reviews.DisplayName != "GitHub Reviews" || reviews.DeletedAt != nil {
		t.Fatalf("updated code-review projection = %+v", reviews)
	}
	var slack model.Integration
	if err := db.Unscoped().Where("unique_key = ?", "slack-production").First(&slack).Error; err != nil {
		t.Fatalf("load unavailable Slack integration: %v", err)
	}
	if slack.DeletedAt == nil {
		t.Fatal("Slack integration missing from Nango was not marked unavailable")
	}
	if fixture.nonGETRequest {
		t.Fatal("synchronization attempted to mutate Nango")
	}
}

type configuredIntegrationsFixture struct {
	mu            sync.Mutex
	integrations  []nango.Integration
	nonGETRequest bool
}

func (f *configuredIntegrationsFixture) replace(integrations []nango.Integration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.integrations = integrations
}

func (f *configuredIntegrationsFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Method != http.MethodGet {
		f.nonGETRequest = true
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	switch r.URL.Path {
	case "/providers":
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"name": "github-app", "display_name": "GitHub", "auth_mode": "APP"},
			{"name": "slack", "display_name": "Slack", "auth_mode": "OAUTH2"},
		}})
	case "/integrations":
		_ = json.NewEncoder(w).Encode(map[string]any{"data": f.integrations})
	default:
		http.NotFound(w, r)
	}
}
