package agentruntime

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

const defaultWorkspaceRepoDepth = 1

type WorkspaceConfig struct {
	Repos []WorkspaceRepoConfig `json:"repos"`
}

type WorkspaceRepoConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	CloneURL string `json:"clone_url"`
	Depth    int    `json:"depth"`
}

type selectedGitHubRepository struct {
	ID   string
	Name string
}

func BuildWorkspaceConfig(ctx context.Context, deps CompileDeps, agent *model.Agent) (WorkspaceConfig, error) {
	repos, err := loadSelectedGitHubRepositoriesForAgent(ctx, deps.DB, agent)
	if err != nil {
		return WorkspaceConfig{}, err
	}
	out := WorkspaceConfig{Repos: make([]WorkspaceRepoConfig, 0, len(repos))}
	for _, repo := range repos {
		if !isSafeGitHubRepo(repo.ID) || !isSafeRepoDirectory(repo.Name) {
			continue
		}
		out.Repos = append(out.Repos, WorkspaceRepoConfig{
			ID:       repo.ID,
			Name:     repo.Name,
			FullName: repo.ID,
			CloneURL: "https://github.com/" + repo.ID + ".git",
			Depth:    defaultWorkspaceRepoDepth,
		})
	}
	return out, nil
}

func loadSelectedGitHubRepositoriesForAgent(ctx context.Context, db *gorm.DB, agent *model.Agent) ([]selectedGitHubRepository, error) {
	if db == nil || agent == nil || agent.ID == uuid.Nil {
		return nil, nil
	}
	if agent.OrgID == nil {
		var loaded model.Agent
		err := db.WithContext(ctx).Where("id = ?", agent.ID).First(&loaded).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, nil
			}
			return nil, fmt.Errorf("load agent github resources: %w", err)
		}
		agent = &loaded
	}
	if agent.OrgID == nil {
		return nil, nil
	}

	providers := []string{"github-app", "github-app-code-reviews"}
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

	repos := make([]selectedGitHubRepository, 0)
	for _, conn := range conns {
		selected, err := selectedGitHubRepositoriesFromResources(connectionaccess.EffectiveResources(agent.Resources, conn))
		if err != nil {
			return nil, err
		}
		repos = append(repos, selected...)
	}
	return dedupeSelectedGitHubRepositories(repos), nil
}

func selectedGitHubRepositoriesFromResources(resources model.JSON) ([]selectedGitHubRepository, error) {
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
	return dedupeSelectedGitHubRepositories(repos), nil
}

func selectedGitHubRepositoriesFromRaw(raw any) ([]selectedGitHubRepository, error) {
	switch items := raw.(type) {
	case []any:
		repos := make([]selectedGitHubRepository, 0, len(items))
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
		repos := make([]selectedGitHubRepository, 0, len(items))
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

func selectedGitHubRepositoryFromMap(item map[string]any) (selectedGitHubRepository, bool) {
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
		return selectedGitHubRepository{}, false
	}
	return selectedGitHubRepository{ID: id, Name: name}, true
}

func stringMapValue(item map[string]any, key string) string {
	value, _ := item[key].(string)
	return value
}

func dedupeSelectedGitHubRepositories(repos []selectedGitHubRepository) []selectedGitHubRepository {
	if len(repos) < 2 {
		return repos
	}
	seen := make(map[string]struct{}, len(repos))
	out := make([]selectedGitHubRepository, 0, len(repos))
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
