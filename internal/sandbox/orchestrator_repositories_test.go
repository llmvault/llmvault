package sandbox

import (
	"testing"

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
