package agentruntime

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestSelectedGitHubRepositoriesFromResourcesUsesResourceTypeMap(t *testing.T) {
	repos, err := selectedGitHubRepositoriesFromResources(model.JSON{
		"repository": []any{
			map[string]any{"id": "usehivy/hivy", "name": "hivy", "type": "repository"},
			map[string]any{"id": "usehivy/hivy", "name": "hivy", "type": "repository"},
			map[string]any{"id": "usehivy/worker", "name": "worker", "type": "repository", "full_name": "usehivy/worker"},
		},
	})
	if err != nil {
		t.Fatalf("selected repositories: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("repos=%d, want 2: %+v", len(repos), repos)
	}
	if repos[0].ID != "usehivy/hivy" || repos[0].Name != "hivy" {
		t.Fatalf("first repo=%+v, want usehivy/hivy", repos[0])
	}
	if repos[1].ID != "usehivy/worker" || repos[1].Name != "worker" {
		t.Fatalf("second repo=%+v, want usehivy/worker", repos[1])
	}
}

func TestBuildAgentRuntimeConfigUpdateIncludesWorkspaceRepos(t *testing.T) {
	db := connectCompileTestDB(t)
	agent := createCompileTokenAgent(t, db)
	userID := uuid.New()
	if err := db.Create(&model.User{ID: userID, Email: "repo-config-" + uuid.NewString()[:8] + "@example.com"}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	installGitHubPlugin(t, db, *agent.OrgID, agent.TeamID, "github-app")
	integrationID := uuid.New()
	if err := db.Create(&model.Integration{
		ID:          integrationID,
		UniqueKey:   "github-" + uuid.NewString()[:8],
		Provider:    "github-app",
		DisplayName: "GitHub",
	}).Error; err != nil {
		t.Fatalf("create integration: %v", err)
	}
	if err := db.Create(&model.Connection{
		ID:                uuid.New(),
		OrgID:             *agent.OrgID,
		UserID:            userID,
		IntegrationID:     integrationID,
		NangoConnectionID: "nango-github",
		Meta: model.JSON{
			"resources": model.JSON{
				"repository": []any{
					map[string]any{"id": "usehivy/hivy", "name": "hivy"},
					map[string]any{"id": "usehivy/hivy", "name": "hivy"},
					map[string]any{"id": "bad repo", "name": "../bad"},
				},
			},
		},
	}).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}

	token, err := MintProxyToken(context.Background(), CompileDeps{
		DB:         db,
		SigningKey: []byte("test-signing-key-32-bytes-long!!"),
	}, &agent, uuid.Nil)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	configUpdate, err := BuildAgentRuntimeConfigUpdateWithProxyToken(context.Background(), CompileDeps{
		DB:         db,
		Cfg:        &config.Config{MCPBaseURL: "https://mcp.hivy.test"},
		SigningKey: []byte("test-signing-key-32-bytes-long!!"),
	}, &agent, nil, "runtime-secret", token)
	if err != nil {
		t.Fatalf("build config: %v", err)
	}
	if configUpdate.Workspace == nil {
		t.Fatal("workspace config missing")
	}
	if len(configUpdate.Workspace.Repos) != 1 {
		t.Fatalf("workspace repos=%d, want 1: %+v", len(configUpdate.Workspace.Repos), configUpdate.Workspace.Repos)
	}
	repo := configUpdate.Workspace.Repos[0]
	if repo.ID != "usehivy/hivy" || repo.FullName != "usehivy/hivy" || repo.Name != "hivy" {
		t.Fatalf("repo descriptor = %+v", repo)
	}
	if repo.CloneURL != "https://github.com/usehivy/hivy.git" {
		t.Fatalf("clone_url = %q", repo.CloneURL)
	}
	if repo.Depth != 1 {
		t.Fatalf("depth = %d, want 1", repo.Depth)
	}
}

// installGitHubPlugin grants the GitHub plugin to a team only (org-install +
// team_plugins, NO per-agent row). This is the production-incident regression:
// a team grant alone must resolve the plugin into every team agent's effective
// set, so repo clone config is emitted.
func installGitHubPlugin(t *testing.T, db *gorm.DB, orgID uuid.UUID, teamID uuid.UUID, provider string) {
	t.Helper()
	pluginID := uuid.New()
	if err := db.Create(&model.Plugin{
		ID:       pluginID,
		Slug:     "repo-config-" + provider + "-" + uuid.NewString()[:8],
		Name:     "GitHub",
		Status:   model.PluginStatusActive,
		Manifest: model.RawJSON(`{}`),
	}).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	if err := db.Create(&model.PluginIntegration{
		PluginID: pluginID,
		Provider: provider,
		Kind:     model.PluginIntegrationKindIntegration,
		Required: true,
	}).Error; err != nil {
		t.Fatalf("create plugin integration: %v", err)
	}
	if err := db.Create(&model.OrgPluginInstall{OrgID: orgID, PluginID: pluginID}).Error; err != nil {
		t.Fatalf("create org plugin install: %v", err)
	}
	if err := db.Create(&model.TeamPlugin{OrgID: orgID, TeamID: teamID, PluginID: pluginID}).Error; err != nil {
		t.Fatalf("grant plugin to team: %v", err)
	}
}
