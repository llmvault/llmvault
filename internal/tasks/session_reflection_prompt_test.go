package tasks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/trigger/hivy"
)

func TestGenerateSessionReflectionRequestsStrictJSONSchema(t *testing.T) {
	mock := hivy.NewMockCompletionClient()
	mock.SetFallback(hivy.CompletionResponse{Message: hivy.Message{Content: `{"memories":[]}`}})

	if _, _, err := generateSessionReflection(context.Background(), mock, "openai/gpt-5.4-mini", 0.1, "transcript", "", ""); err != nil {
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
	memories, ok := properties["memories"].(map[string]any)
	if !ok {
		t.Fatalf("schema missing memories property: %#v", schema["properties"])
	}
	items := memories["items"].(map[string]any)
	itemProps := items["properties"].(map[string]any)
	for _, field := range []string{"content", "kind", "tags", "confidence", "entities", "expires_at", "source_event_ids", "actor_display_name", "actor_external_ref"} {
		if _, ok := itemProps[field]; !ok {
			t.Fatalf("schema missing memory field %q: %#v", field, itemProps)
		}
	}
	kindEnum := itemProps["kind"].(map[string]any)["enum"].([]any)
	wantKinds := []string{"preference", "rule", "decision", "convention", "org-fact", "person", "workaround", "commitment", "finding"}
	if len(kindEnum) != len(wantKinds) {
		t.Fatalf("kind enum=%#v want %v", kindEnum, wantKinds)
	}
	for index, kind := range wantKinds {
		if kindEnum[index] != kind {
			t.Fatalf("kind enum[%d]=%v want %s", index, kindEnum[index], kind)
		}
	}
}

func TestBuildSessionReflectionSystemPromptStructureAndMissionSeam(t *testing.T) {
	prompt := buildSessionReflectionSystemPrompt("")
	for _, marker := range []string{
		"False positives are much more harmful than omissions",
		"A candidate is a memory only when EVERY gate below passes:",
		"Role: Agent text is never authoritative evidence",
		"ALWAYS RETURN NO MEMORY FOR:",
		"Tool catalogs, tool names, available commands",
		"Inventories of Slack channels",
		"Successful hello/test messages",
		"NON-DERIVABILITY",
		"Before emitting each candidate, try to disqualify it",
		"EXAMPLES:",
		`{"memories":[]}`,
	} {
		if !strings.Contains(prompt, marker) {
			t.Fatalf("prompt missing %q:\n%s", marker, prompt)
		}
	}
	if strings.Contains(prompt, "FOCUS —") {
		t.Fatalf("prompt without mission must omit the FOCUS section:\n%s", prompt)
	}

	withMission := buildSessionReflectionSystemPrompt("Retain everything durable about ACME.")
	if !strings.Contains(withMission, "FOCUS — what to retain for this agent (takes priority over the general guidelines):\nRetain everything durable about ACME.") {
		t.Fatalf("prompt with mission missing FOCUS section:\n%s", withMission)
	}
}

func TestGenerateSessionReflectionCapsMemoriesPerRun(t *testing.T) {
	memories := make([]string, 0, sessionReflectionMaxMemories+3)
	for range sessionReflectionMaxMemories + 3 {
		memories = append(memories, `{
			"content": "The team deploys the API on Railway.",
			"kind": "org-fact",
			"tags": ["infra"],
			"confidence": 0.9,
			"entities": ["Railway"],
			"expires_at": "",
			"source_event_ids": [],
			"actor_display_name": "",
			"actor_external_ref": ""
		}`)
	}
	mock := hivy.NewMockCompletionClient()
	mock.SetFallback(hivy.CompletionResponse{Message: hivy.Message{
		Content: `{"memories":[` + strings.Join(memories, ",") + `]}`,
	}})
	result, _, err := generateSessionReflection(context.Background(), mock, "openai/gpt-5.4-mini", 0.1, "transcript", "", "")
	if err != nil {
		t.Fatalf("generate reflection: %v", err)
	}
	if len(result.Memories) != sessionReflectionMaxMemories {
		t.Fatalf("memories len=%d want cap %d", len(result.Memories), sessionReflectionMaxMemories)
	}
}

func TestParseSessionReflectionResponseFiltersInvalidAndUnsafeMemories(t *testing.T) {
	result, err := parseSessionReflectionResponse(`{
		"memories": [
			{
				"content": "Dana prefers concise implementation plans.",
				"kind": "preference",
				"confidence": 0.9,
				"entities": [" Dana ", ""],
				"expires_at": "2026-12-31",
				"source_event_ids": ["11111111-1111-1111-1111-111111111111"]
			},
			{
				"content": "The API key is sk-test",
				"kind": "finding",
				"confidence": 0.9
			},
			{
				"content": "Low confidence guess about tooling.",
				"kind": "finding",
				"confidence": 0.84
			},
			{
				"content": "Bad kind",
				"kind": "mood",
				"confidence": 0.9
			},
			{
				"content": "The sandbox has 3.9 GB of disk space.",
				"kind": "environment",
				"confidence": 0.95
			},
			{
				"content": "Invalid expiry is cleared, memory kept.",
				"kind": "commitment",
				"confidence": 0.9,
				"expires_at": "soon"
			}
		]
	}`)
	if err != nil {
		t.Fatalf("parse reflection response: %v", err)
	}
	if len(result.Memories) != 2 {
		t.Fatalf("memories len=%d want 2: %#v", len(result.Memories), result.Memories)
	}
	first := result.Memories[0]
	if first.Content != "Dana prefers concise implementation plans." {
		t.Fatalf("content=%q", first.Content)
	}
	if len(first.Entities) != 1 || first.Entities[0] != "Dana" {
		t.Fatalf("entities=%#v", first.Entities)
	}
	if first.ExpiresAt != "2026-12-31" {
		t.Fatalf("expires_at=%q", first.ExpiresAt)
	}
	second := result.Memories[1]
	if second.Content != "Invalid expiry is cleared, memory kept." || second.ExpiresAt != "" {
		t.Fatalf("second memory=%#v", second)
	}
}

func TestParseSessionReflectionResponseRejectsInvalidJSON(t *testing.T) {
	if _, err := parseSessionReflectionResponse(`not-json`); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}
