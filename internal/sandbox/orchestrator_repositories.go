package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func (o *Orchestrator) cloneAgentRepositories(ctx context.Context, sb *model.Sandbox, agent *model.Agent) error {
	if len(agent.Resources) == 0 {
		return nil
	}

	var repos []repoResource
	for _, resourceTypes := range agent.Resources {
		typesMap, ok := resourceTypes.(map[string]any)
		if !ok {
			continue
		}
		repoList, ok := typesMap["repository"]
		if !ok {
			continue
		}
		repoSlice, ok := repoList.([]any)
		if !ok {
			continue
		}
		for _, item := range repoSlice {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			repoID, _ := itemMap["id"].(string)
			repoName, _ := itemMap["name"].(string)
			if repoID != "" && repoName != "" {
				repos = append(repos, repoResource{ID: repoID, Name: repoName})
			}
		}
	}

	if len(repos) == 0 {
		return nil
	}
	return o.cloneRepositories(ctx, sb, repos, o.runtimeLayout().WorkspaceRepoDir)
}

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

	raw := selectedRepositoriesFromResources(agent.Resources)
	if raw == nil {
		return nil, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal selected github repositories: %w", err)
	}
	var selected []struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"`
	}
	if err := json.Unmarshal(data, &selected); err != nil {
		return nil, fmt.Errorf("decode selected github repositories: %w", err)
	}

	repos := make([]repoResource, 0, len(selected))
	for _, repo := range selected {
		name := strings.TrimSpace(repo.Name)
		fullName := strings.TrimSpace(repo.FullName)
		if name == "" || fullName == "" {
			continue
		}
		repos = append(repos, repoResource{ID: fullName, Name: name})
	}
	return repos, nil
}

func selectedRepositoriesFromResources(resources model.JSON) any {
	for _, resourceTypes := range resources {
		typesMap, ok := resourceTypes.(map[string]any)
		if !ok {
			continue
		}
		if repos := typesMap["repository"]; repos != nil {
			return repos
		}
	}
	return nil
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
