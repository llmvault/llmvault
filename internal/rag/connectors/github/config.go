package github

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/rag/connectors/scope"
)

type GithubConfig struct {
	RepoOwner     string   `json:"repo_owner"`
	Repositories  []string `json:"repositories,omitempty"`
	StateFilter   string   `json:"state_filter,omitempty"`
	IncludePRs    bool     `json:"include_prs"`
	IncludeIssues bool     `json:"include_issues"`

	// FullNames is the resolved set of "owner/repo" identifiers to index,
	// computed by LoadConfig from either the scope envelope (multi-owner) or
	// the legacy RepoOwner+Repositories fields. Not part of the JSON contract.
	FullNames []string `json:"-"`
}

// githubScopeResourceType is the resource-catalog key for a GitHub repo scope.
const githubScopeResourceType = "repository"

// validStateFilters: "merged" is a frequent admin mistake — it's not a
// real GitHub state (merged PRs surface as closed), so we reject it
// explicitly to fail fast at create time rather than silently return
// empty fetches at run time.
var validStateFilters = map[string]struct{}{
	"open":   {},
	"closed": {},
	"all":    {},
}

func LoadConfig(raw json.RawMessage) (GithubConfig, error) {
	cfg := GithubConfig{
		IncludePRs:    true,
		IncludeIssues: true,
	}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return GithubConfig{}, fmt.Errorf("github: parse config: %w", err)
		}
	}

	cfg.StateFilter = strings.ToLower(strings.TrimSpace(cfg.StateFilter))
	if cfg.StateFilter == "" {
		cfg.StateFilter = "all"
	}
	if _, ok := validStateFilters[cfg.StateFilter]; !ok {
		return GithubConfig{}, fmt.Errorf(
			"github: state_filter %q is not one of {open, closed, all}", cfg.StateFilter,
		)
	}

	fullNames, err := resolveRepoFullNames(raw, cfg)
	if err != nil {
		return GithubConfig{}, err
	}
	cfg.FullNames = fullNames
	cfg.Repositories = normaliseRepoList(cfg.Repositories)
	return cfg, nil
}

// resolveRepoFullNames computes the "owner/repo" set to index. Preference order:
//  1. the scope envelope (config.scope, resource_type "repository") — each item
//     id is already a full "owner/repo", so multiple owners are supported.
//  2. the legacy RepoOwner + Repositories fields (single owner).
//
// An empty result is allowed here (mirrors the original parse-time leniency);
// the "at least one repository" requirement is enforced at LoadFromCheckpoint.
func resolveRepoFullNames(raw json.RawMessage, cfg GithubConfig) ([]string, error) {
	sc, present, err := scope.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("github: %w", err)
	}
	if present && sc.ResourceType == githubScopeResourceType && len(sc.Items) > 0 {
		full := make([]string, 0, len(sc.Items))
		for _, id := range sc.IDs() {
			if !strings.Contains(id, "/") {
				return nil, fmt.Errorf("github: scope repository %q must be in owner/repo form", id)
			}
			full = append(full, id)
		}
		return full, nil
	}

	owner := strings.TrimSpace(cfg.RepoOwner)
	if owner == "" {
		return nil, fmt.Errorf("github: repo_owner is required (or provide a repository scope)")
	}
	repos := normaliseRepoList(cfg.Repositories)
	full := make([]string, 0, len(repos))
	for _, r := range repos {
		full = append(full, owner+"/"+r)
	}
	return full, nil
}

// normaliseRepoList re-splits comma-joined entries — admins occasionally
// type "a,b,c" into a JSON array of one string instead of three.
func normaliseRepoList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, entry := range in {
		for _, part := range strings.Split(entry, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
