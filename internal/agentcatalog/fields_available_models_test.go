package agentcatalog

import (
	"reflect"
	"testing"

	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/model"
)

func TestCatalogUpdatesNormalizesAvailableModels(t *testing.T) {
	updates := catalogUpdates(Manifest{
		Runtime: RuntimeManifest{
			Model: "deepseek-v4-flash",
			AvailableModels: []string{
				" gemini-3-flash-preview ",
				"",
				"deepseek-v4-flash",
				"gemini-3-flash-preview",
			},
		},
	}, model.RawJSON("{}"), "hash", model.AgentCatalogStatusActive)

	got, ok := updates["available_models"].(pq.StringArray)
	if !ok {
		t.Fatalf("available_models has type %T", updates["available_models"])
	}
	want := []string{"gemini-3-flash-preview", "deepseek-v4-flash"}
	if !reflect.DeepEqual([]string(got), want) {
		t.Fatalf("available_models = %#v, want %#v", got, want)
	}
}
