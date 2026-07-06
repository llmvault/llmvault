package resources

import (
	"encoding/json"
	"testing"

	"github.com/usehivy/hivy/internal/mcp/catalog"
)

func unmarshal(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func TestExtractResource_TopLevelString(t *testing.T) {
	obj := unmarshal(t, `{"full_name":"acme/web","name":"web"}`)
	def := &catalog.ResourceDef{IDField: "full_name", NameField: "name"}
	got := extractResource(obj, "repository", def)
	if got.ID != "acme/web" || got.Name != "web" {
		t.Fatalf("got %+v, want id=acme/web name=web", got)
	}
}

func TestExtractResource_NotionPage(t *testing.T) {
	obj := unmarshal(t, `{
		"object": "page",
		"id": "page-123",
		"properties": {
			"Name": {"id":"title","type":"title","title":[{"plain_text":"Engineering "},{"plain_text":"Wiki"}]}
		}
	}`)
	def := &catalog.ResourceDef{IDField: "id", NameField: "title"}
	got := extractResource(obj, "page", def)
	if got.ID != "page-123" {
		t.Fatalf("id = %q, want page-123", got.ID)
	}
	if got.Name != "Engineering Wiki" {
		t.Fatalf("name = %q, want %q (nested page title)", got.Name, "Engineering Wiki")
	}
}

func TestExtractResource_NotionDataSource(t *testing.T) {
	obj := unmarshal(t, `{
		"object": "data_source",
		"id": "ds-456",
		"title": [{"plain_text":"Decision Log"}]
	}`)
	def := &catalog.ResourceDef{IDField: "id", NameField: "title"}
	got := extractResource(obj, "data_source", def)
	if got.ID != "ds-456" {
		t.Fatalf("id = %q, want ds-456", got.ID)
	}
	if got.Name != "Decision Log" {
		t.Fatalf("name = %q, want %q (rich-text array title)", got.Name, "Decision Log")
	}
}

func TestExtractName_EmptyWhenNoTitle(t *testing.T) {
	obj := unmarshal(t, `{"object":"page","id":"x","properties":{}}`)
	if got := extractName(obj, "title"); got != "" {
		t.Fatalf("extractName = %q, want empty", got)
	}
}
