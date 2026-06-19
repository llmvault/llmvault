package sandbox

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/connectionaccess"
	"github.com/usehivy/hivy/internal/model"
)

func (o *Orchestrator) cloneAgentSelectedRepositories(ctx context.Context, sb *model.Sandbox, agent *model.Agent) error {
	return o.SyncAgentSelectedRepositories(ctx, sb, agent)
}

func (o *Orchestrator) SyncAgentSelectedRepositories(ctx context.Context, sb *model.Sandbox, agent *model.Agent) error {
	repos, err := o.loadAgentSelectedGitHubRepositories(ctx, agent)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return nil
	}
	return o.cloneRepositories(ctx, sb, repos, o.runtimeLayout().AgentRepoDir)
}

func (o *Orchestrator) SyncGitHubConnectionResources(ctx context.Context, sb *model.Sandbox, resources model.JSON) error {
	repos, err := selectedGitHubRepositoriesFromResources(resources)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return nil
	}
	return o.cloneRepositories(ctx, sb, repos, o.runtimeLayout().AgentRepoDir)
}

func (o *Orchestrator) loadAgentSelectedGitHubRepositories(ctx context.Context, agent *model.Agent) ([]repoResource, error) {
	if agent == nil {
		return nil, nil
	}
	return loadSelectedGitHubRepositoriesForAgent(ctx, o.db, agent.ID)
}

func loadSelectedGitHubRepositoriesForAgent(ctx context.Context, db *gorm.DB, agentID uuid.UUID) ([]repoResource, error) {
	if db == nil {
		return nil, nil
	}
	var agent model.Agent
	err := db.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("load agent github resources: %w", err)
	}
	if agent.OrgID == nil {
		return nil, nil
	}

	providers := []string{"github-app", "github-app-oauth", "github", "github-pat"}
	var conns []model.Connection
	if err := db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Joins("JOIN plugin_integrations ON plugin_integrations.provider = integrations.provider AND plugin_integrations.kind = ?", model.PluginIntegrationKindIntegration).
		Joins("JOIN agent_plugin_installs ON agent_plugin_installs.plugin_id = plugin_integrations.plugin_id AND agent_plugin_installs.org_id = connections.org_id AND agent_plugin_installs.agent_id = ?", agent.ID).
		Joins("JOIN org_plugin_installs ON org_plugin_installs.plugin_id = plugin_integrations.plugin_id AND org_plugin_installs.org_id = connections.org_id AND org_plugin_installs.revoked_at IS NULL").
		Joins("JOIN plugins ON plugins.id = plugin_integrations.plugin_id AND plugins.status = ?", model.PluginStatusActive).
		Where("connections.org_id = ? AND connections.revoked_at IS NULL AND integrations.provider IN ?", *agent.OrgID, providers).
		Order("connections.created_at ASC").
		Find(&conns).Error; err != nil {
		return nil, fmt.Errorf("load agent github connections: %w", err)
	}

	var repos []repoResource
	for _, conn := range conns {
		selected, err := selectedGitHubRepositoriesFromResources(connectionaccess.EffectiveResources(agent.Resources, conn))
		if err != nil {
			return nil, err
		}
		repos = append(repos, selected...)
	}
	return dedupeRepoResources(repos), nil
}

func selectedGitHubRepositoriesFromResources(resources model.JSON) ([]repoResource, error) {
	if len(resources) == 0 {
		return nil, nil
	}
	raw, ok := resources["repository"]
	if !ok || raw == nil {
		return nil, nil
	}
	repos, err := selectedGitHubRepositoriesFromRaw(raw)
	if err != nil {
		return nil, err
	}
	return dedupeRepoResources(repos), nil
}

func selectedGitHubRepositoriesFromRaw(raw any) ([]repoResource, error) {
	switch items := raw.(type) {
	case []any:
		repos := make([]repoResource, 0, len(items))
		for _, item := range items {
			itemMap, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("selected github repository has invalid shape")
			}
			if repo, ok := selectedGitHubRepositoryFromMap(itemMap); ok {
				repos = append(repos, repo)
			}
		}
		return repos, nil
	case []map[string]any:
		repos := make([]repoResource, 0, len(items))
		for _, item := range items {
			if repo, ok := selectedGitHubRepositoryFromMap(item); ok {
				repos = append(repos, repo)
			}
		}
		return repos, nil
	default:
		return nil, fmt.Errorf("selected github repositories have invalid shape")
	}
}

func selectedGitHubRepositoryFromMap(item map[string]any) (repoResource, bool) {
	id := strings.TrimSpace(stringMapValue(item, "full_name"))
	if id == "" {
		id = strings.TrimSpace(stringMapValue(item, "id"))
	}
	name := strings.TrimSpace(stringMapValue(item, "name"))
	if name == "" && strings.Contains(id, "/") {
		parts := strings.Split(id, "/")
		name = parts[len(parts)-1]
	}
	if id == "" || name == "" {
		return repoResource{}, false
	}
	return repoResource{ID: id, Name: name}, true
}

func stringMapValue(item map[string]any, key string) string {
	value, _ := item[key].(string)
	return value
}

func dedupeRepoResources(repos []repoResource) []repoResource {
	if len(repos) < 2 {
		return repos
	}
	seen := make(map[string]struct{}, len(repos))
	out := make([]repoResource, 0, len(repos))
	for _, repo := range repos {
		key := strings.TrimSpace(repo.ID)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, repo)
	}
	return out
}

func (o *Orchestrator) cloneRepositories(ctx context.Context, sb *model.Sandbox, repos []repoResource, baseDir string) error {
	if _, err := o.ExecuteCommand(ctx, sb, "mkdir -p "+shellQuote(baseDir)); err != nil {
		return fmt.Errorf("creating repos directory: %w", err)
	}
	for _, repo := range repos {
		if !isSafeGitHubRepo(repo.ID) {
			return fmt.Errorf("invalid GitHub repository full name %q", repo.ID)
		}
		if !isSafeRepoDirectory(repo.Name) {
			return fmt.Errorf("invalid repository directory name %q", repo.Name)
		}
		repoPath := baseDir + "/" + repo.Name
		cloneURL := "https://github.com/" + repo.ID + ".git"

		command := fmt.Sprintf("if [ -d %s ]; then if [ -d %s ]; then git -C %s fetch --depth=1 origin; else echo 'repository path exists but is not a git checkout' >&2; exit 1; fi; else git clone --depth=1 %s %s; fi",
			shellQuote(repoPath),
			shellQuote(repoPath+"/.git"),
			shellQuote(repoPath),
			shellQuote(cloneURL),
			shellQuote(repoPath),
		)
		if _, err := o.ExecuteCommand(ctx, sb, command); err != nil {
			return fmt.Errorf("cloning %s: %w", repo.ID, err)
		}
	}
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func isSafeGitHubRepo(value string) bool {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return false
	}
	return isSafeRepoDirectory(parts[0]) && isSafeRepoDirectory(parts[1])
}

func isSafeRepoDirectory(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}
