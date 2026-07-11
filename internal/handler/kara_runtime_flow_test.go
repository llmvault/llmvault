package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentcatalog"
	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	pluginstore "github.com/usehivy/hivy/internal/plugins"
	"github.com/usehivy/hivy/internal/registry"
	runtimeapi "github.com/usehivy/hivy/internal/sandboxruntime"
)

func TestGlobalKaraCatalogInstallCompilesRuntimeFilters(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	seedDefaultModelCredential(t, db)
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.TeamPlugin{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgPluginInstall{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
	})

	if _, err := pluginstore.SyncLocal(ctx, db, "global/plugins"); err != nil {
		t.Fatalf("sync global plugins: %v", err)
	}
	if _, err := agentcatalog.SyncLocal(ctx, db, "global/agents"); err != nil {
		t.Fatalf("sync global agents: %v", err)
	}

	var design model.Plugin
	if err := db.Where("slug = ? AND status = ?", "design", model.PluginStatusActive).First(&design).Error; err != nil {
		t.Fatalf("load design plugin: %v", err)
	}
	if err := db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: org.ID, PluginID: design.ID}).Error; err != nil {
		t.Fatalf("install design plugin for org: %v", err)
	}

	// Catalog agents install into a team; the team must have the required
	// (non-auto-install) design plugin provisioned for it.
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	t.Cleanup(func() {
		db.Where("team_id = ?", team.ID).Delete(&model.TeamPlugin{})
		db.Where("id = ?", team.ID).Delete(&model.Team{})
	})
	if err := db.Create(&model.TeamPlugin{ID: uuid.New(), OrgID: org.ID, TeamID: team.ID, PluginID: design.ID}).Error; err != nil {
		t.Fatalf("provision design plugin for team: %v", err)
	}

	h := handler.NewAgentHandler(db, nil, agentruntime.CompileDeps{}, registry.Global())
	r := chi.NewRouter()
	r.Post("/v1/agents/catalog/{slug}/install", h.InstallCatalog)
	installBody := bytes.NewReader([]byte(`{"team_id":"` + team.ID.String() + `"}`))
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/catalog/kara-ui-and-graphics-designer/install", installBody)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}

	var catalog model.AgentCatalog
	if err := db.Where("slug = ?", "kara-ui-and-graphics-designer").First(&catalog).Error; err != nil {
		t.Fatalf("load kara catalog: %v", err)
	}
	var agent model.Agent
	if err := db.Preload("AgentCatalog").
		Where("org_id = ? AND agent_catalog_id = ?", org.ID, catalog.ID).
		First(&agent).Error; err != nil {
		t.Fatalf("load installed kara agent: %v", err)
	}

	proxyToken := &agentruntime.ProxyTokenResult{Token: "ptok_test", JTI: uuid.NewString()}
	def, err := agentruntime.CompileWithProxyToken(ctx, agentruntime.CompileDeps{DB: db, Cfg: &config.Config{}}, &agent, proxyToken)
	if err != nil {
		t.Fatalf("compile kara runtime definition: %v", err)
	}

	// Every compiled MCP filter gets skill_view so an allow-listed sub-agent can
	// load the skills named in its system prompt.
	imageToolsWithFloor := []string{
		"generate_image", "generate_vector_image", "remix_image", "skill_view",
	}

	assertToolFilter(t, "kara", def.McpToolFilter, []string{"skill_view"}, nil)

	imageGenerator := def.SubAgents["image-generator"]
	if imageGenerator == nil {
		t.Fatalf("missing image-generator subagent; got %s", strings.Join(sortedSubAgentKeys(def.SubAgents), ", "))
	}
	assertToolFilter(t, "image-generator", imageGenerator.McpToolFilter, imageToolsWithFloor, nil)
	// image-generator configures no runtime tools; empty sub-agent selections
	// default to read_file instead of compiling tool-less.
	if got := len(imageGenerator.Tools); got != 1 || imageGenerator.Tools[0]["type"] != "builtin.read_file" {
		t.Fatalf("image-generator tools = %#v, want default read_file", imageGenerator.Tools)
	}

	designWorker := def.SubAgents["design-worker"]
	if designWorker == nil {
		t.Fatalf("missing design-worker subagent; got %s", strings.Join(sortedSubAgentKeys(def.SubAgents), ", "))
	}
	assertToolFilter(t, "design-worker", designWorker.McpToolFilter, []string{"skill_view"}, nil)

	body, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal compiled definition: %v", err)
	}
	var runtimeDef runtimeapi.AgentDefinition
	if err := json.Unmarshal(body, &runtimeDef); err != nil {
		t.Fatalf("compiled definition does not match runtime API shape: %v", err)
	}
	assertRuntimeAPIFilter(t, "runtime kara mcp", runtimeDef.McpToolFilter, []string{"skill_view"}, nil)
	if runtimeDef.SubAgents == nil {
		t.Fatal("runtime payload missing subagents")
	}
	runtimeImageGenerator, ok := (*runtimeDef.SubAgents)["image-generator"]
	if !ok {
		t.Fatalf("runtime payload missing image-generator subagent")
	}
	assertRuntimeAPIFilter(t, "runtime image-generator mcp", runtimeImageGenerator.McpToolFilter, imageToolsWithFloor, nil)
}

func assertToolFilter(t *testing.T, label string, filter *model.ToolFilter, wantAllow, wantDeny []string) {
	t.Helper()
	if filter == nil {
		t.Fatalf("%s tool filter is nil", label)
	}
	if !sameOptionalStrings(filter.Allow, wantAllow) || !sameOptionalStrings(filter.Deny, wantDeny) {
		t.Fatalf("%s tool filter = %#v, want allow=%#v deny=%#v", label, filter, wantAllow, wantDeny)
	}
}

func assertRuntimeToolTypes(t *testing.T, label string, tools []map[string]any, want []string) {
	t.Helper()
	got := make([]string, 0, len(tools))
	for _, tool := range tools {
		value, ok := tool["type"].(string)
		if !ok {
			t.Fatalf("%s tool has non-string type: %#v", label, tool)
		}
		got = append(got, value)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s tools = %#v, want %#v", label, got, want)
	}
}

func assertRuntimeAPIFilter(t *testing.T, label string, filter *runtimeapi.ToolFilter, wantAllow, wantDeny []string) {
	t.Helper()
	if filter == nil {
		t.Fatalf("%s filter is nil", label)
	}
	if !sameOptionalStringPointer(filter.Allow, wantAllow) || !sameOptionalStringPointer(filter.Deny, wantDeny) {
		t.Fatalf("%s filter = %#v, want allow=%#v deny=%#v", label, filter, wantAllow, wantDeny)
	}
}

func sameOptionalStringPointer(got *[]string, want []string) bool {
	if got == nil {
		return want == nil
	}
	return sameOptionalStrings(*got, want)
}

func sameOptionalStrings(got, want []string) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return reflect.DeepEqual(got, want)
}

func sortedSubAgentKeys(subAgents map[string]*agentruntime.AgentDefinition) []string {
	keys := make([]string, 0, len(subAgents))
	for key := range subAgents {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
