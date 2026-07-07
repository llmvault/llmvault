package tasks

import (
	"encoding/json"
	"testing"

	"github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
)

func TestScopeSourceConfigToEntities_Integration(t *testing.T) {
	src := &ragmodel.RAGSource{
		ConfigValue: model.JSON{
			"scope": map[string]any{
				"resource_type": "",
				"items": []any{
					map[string]any{"id": "acme/web", "type": "repository"},
					map[string]any{"id": "acme/api", "type": "repository"},
					map[string]any{"id": "acme/cli", "type": "repository"},
				},
			},
		},
	}

	scopeSourceConfigToEntities(src, []string{"acme/api"})

	scope := src.ConfigValue["scope"].(map[string]any)
	items := scope["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if id := items[0].(map[string]any)["id"]; id != "acme/api" {
		t.Fatalf("kept id = %v, want acme/api", id)
	}
}

func TestScopeSourceConfigToEntities_WebsiteURLs(t *testing.T) {
	src := &ragmodel.RAGSource{
		ConfigValue: model.JSON{
			"urls": []any{"https://a.test/x", "https://a.test/y", "https://a.test/z"},
		},
	}

	scopeSourceConfigToEntities(src, []string{"https://a.test/y"})

	raw, _ := json.Marshal(src.ConfigValue["urls"])
	if string(raw) != `["https://a.test/y"]` {
		t.Fatalf("urls = %s, want [\"https://a.test/y\"]", raw)
	}
}
