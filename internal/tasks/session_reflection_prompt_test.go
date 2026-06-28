package tasks

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/usehivy/hivy/internal/trigger/hivy"
)

func TestGenerateSessionReflectionRequestsStrictJSONSchema(t *testing.T) {
	mock := hivy.NewMockCompletionClient()
	mock.SetFallback(hivy.CompletionResponse{Message: hivy.Message{Content: `{"memories":[]}`}})

	if _, err := generateSessionReflection(context.Background(), mock, "openai/gpt-4o-mini", "transcript", ""); err != nil {
		t.Fatalf("generate reflection: %v", err)
	}

	req := mock.LastRequest()
	if req.ResponseFormat == nil || req.ResponseFormat.Type != hivy.ResponseFormatJSONSchema {
		t.Fatalf("response format=%#v", req.ResponseFormat)
	}
	if req.ResponseFormat.JSONSchema == nil || req.ResponseFormat.JSONSchema.Name != "session_reflection" || !req.ResponseFormat.JSONSchema.Strict {
		t.Fatalf("json schema=%#v", req.ResponseFormat.JSONSchema)
	}

	var schema map[string]any
	if err := json.Unmarshal(req.ResponseFormat.JSONSchema.Schema, &schema); err != nil {
		t.Fatalf("schema json: %v", err)
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("additionalProperties=%#v", schema["additionalProperties"])
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema missing properties: %#v", schema)
	}
	if _, ok := properties["memories"]; !ok {
		t.Fatalf("schema missing memories property: %#v", schema["properties"])
	}
}

func TestParseSessionReflectionResponseFiltersInvalidAndUnsafeMemories(t *testing.T) {
	result, err := parseSessionReflectionResponse(`{
		"memories": [
			{
				"content": "Dana prefers concise implementation plans.",
				"scope": "user",
				"visibility": "all_agents",
				"kind": "preference",
				"confidence": 0.9,
				"source_event_ids": ["11111111-1111-1111-1111-111111111111"]
			},
			{
				"content": "The API key is sk-test",
				"scope": "org",
				"visibility": "all_agents",
				"kind": "system",
				"confidence": 0.9
			},
			{
				"content": "Bad scope",
				"scope": "workspace",
				"visibility": "all_agents",
				"kind": "preference",
				"confidence": 0.9
			},
			{
				"content": "Bad kind",
				"scope": "org",
				"visibility": "all_agents",
				"kind": "mood",
				"confidence": 0.9
			}
		]
	}`)
	if err != nil {
		t.Fatalf("parse reflection response: %v", err)
	}
	if len(result.Memories) != 1 {
		t.Fatalf("memories len=%d want 1: %#v", len(result.Memories), result.Memories)
	}
	if result.Memories[0].Content != "Dana prefers concise implementation plans." {
		t.Fatalf("content=%q", result.Memories[0].Content)
	}
}

func TestParseSessionReflectionResponseRejectsInvalidJSON(t *testing.T) {
	if _, err := parseSessionReflectionResponse(`not-json`); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}
