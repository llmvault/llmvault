package tasks

import "testing"

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
