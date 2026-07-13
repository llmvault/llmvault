package integrations

import "testing"

func TestListSupportedDefinitionsIncludesEnabledManifests(t *testing.T) {
	definitions, err := ListSupportedDefinitions("global/integrations")
	if err != nil {
		t.Fatalf("list supported definitions: %v", err)
	}
	if len(definitions) == 0 {
		t.Fatal("expected supported integration definitions")
	}

	seen := map[string]bool{}
	for _, definition := range definitions {
		if definition.ID == "" || definition.Provider == "" || definition.UniqueKey == "" || definition.DisplayName == "" {
			t.Fatalf("incomplete supported definition: %+v", definition)
		}
		seen[definition.ID] = true
	}
	for _, required := range []string{"slack", "github-app", "github-app-code-reviews"} {
		if !seen[required] {
			t.Fatalf("supported definitions missing %q", required)
		}
	}
}
