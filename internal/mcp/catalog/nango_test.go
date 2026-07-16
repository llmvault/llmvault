package catalog

import "testing"

func TestEmbeddedNangoCatalog(t *testing.T) {
	catalog := Global()

	source, revision := catalog.NangoSource()
	if source != "usehivy/integrations:packages/shared/flows.zero.json" {
		t.Fatalf("unexpected source %q", source)
	}
	if revision == "" {
		t.Fatal("expected source revision")
	}

	slack, ok := catalog.NangoProvider("slack")
	if !ok {
		t.Fatal("expected Slack in embedded Nango catalog")
	}
	for _, action := range slack.Actions {
		if action.Name == "post-message" {
			if len(action.InputSchema) == 0 {
				t.Fatal("expected post-message input schema")
			}
			return
		}
	}
	t.Fatal("expected Slack post-message action")
}
