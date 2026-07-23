package handler

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func connectMCPHandlerFilterTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(testdb.DatabaseURL()), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

// The server cache is keyed by JTI, but the capability set must still come
// from the same compiler policy as the runtime definition. This prevents an
// agent-proxy token from rebuilding the unfiltered native Hivy catalog.
func TestAgentProxyMCPToolFilterUsesCompilerAllowList(t *testing.T) {
	db := connectMCPHandlerFilterTestDB(t)
	org := model.Org{ID: uuid.New(), Name: "mcp-filter-" + uuid.NewString(), Active: true, RateLimit: 1000}
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "mcp-filter-team-" + uuid.NewString()}
	agent := model.Agent{
		ID:                  uuid.New(),
		OrgID:               &org.ID,
		TeamID:              team.ID,
		Name:                "Filtered agent",
		Model:               "test-model",
		Status:              "active",
		EmailInboxLocalPart: "filtered-agent",
		McpToolFilter:       &model.ToolFilter{Allow: []string{"sheet_list"}},
	}
	for _, row := range []any{&org, &team, &agent} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Delete(&model.Org{}, "id = ?", org.ID)
	})

	h := NewMCPHandler(db, nil, nil, nil, nil)
	filter := h.agentProxyMCPToolFilter(context.Background(), &model.Token{
		OrgID: org.ID,
		Meta: model.JSON{
			model.TokenMetaType:    model.TokenTypeAgentProxy,
			model.TokenMetaAgentID: agent.ID.String(),
		},
	})
	if filter == nil {
		t.Fatal("agent proxy filter is nil")
	}
	got := map[string]bool{}
	for _, id := range filter.Allow {
		got[id] = true
	}
	if !got["sheet_list"] || !got["skill_view"] {
		t.Fatalf("compiled JTI filter = %#v, want sheet_list plus skill_view", filter)
	}
	for _, id := range model.AgentEmailMCPToolIDs {
		if !got[id] {
			t.Fatalf("compiled JTI filter = %#v, want inbox-derived tool %q", filter, id)
		}
	}
	if got["app_create"] || got["web_search"] || got["search_knowledge_base"] {
		t.Fatalf("compiled JTI filter leaked ungranted tools: %#v", filter)
	}
}
