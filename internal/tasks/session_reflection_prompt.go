package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/trigger/hivy"
)

const sessionReflectionMaxTokens = 3000

type sessionReflectionResult struct {
	Memories []reflectionMemoryCandidate `json:"memories"`
}

type reflectionMemoryCandidate struct {
	Content          string   `json:"content"`
	Scope            string   `json:"scope"`
	Visibility       string   `json:"visibility"`
	Kind             string   `json:"kind"`
	Tags             []string `json:"tags"`
	Confidence       float64  `json:"confidence"`
	SourceEventIDs   []string `json:"source_event_ids"`
	ActorDisplayName string   `json:"actor_display_name"`
	ActorExternalRef string   `json:"actor_external_ref"`
}

func generateSessionReflection(ctx context.Context, client hivy.CompletionClient, modelID, transcript, existing string) (sessionReflectionResult, error) {
	systemPrompt := strings.Join([]string{
		"You extract durable memories from Hivy agent sessions.",
		"Return strict JSON only in the shape {\"memories\":[...]} with no prose.",
		"Return compact minified JSON and keep each content value concise.",
		"Each memory object must include content, scope, visibility, kind, tags, confidence, source_event_ids, actor_display_name, and actor_external_ref.",
		"Each memory must be one stable fact, preference, rule, identity, environment constraint, project convention, workaround, or decision.",
		"Use scope=user only for a specific user's identity or preferences; use scope=org for organization, project, environment, system, or external-user facts.",
		"Use visibility=this_agent only when the memory is useful only for this agent; otherwise use visibility=all_agents.",
		"Allowed kinds are identity, preference, role, project, system, environment, workaround, constraint, decision, integration, and other.",
		"Use lowercase kebab-case tags and cite source_event_ids from transcript event UUIDs.",
		"Set actor_display_name and actor_external_ref from the transcript Actor line when evidence comes from a human message; use empty strings when unavailable.",
		"Never store secrets, tokens, passwords, private keys, API keys, raw credentials, hidden reasoning, or transient task progress.",
		"Do not guess. If evidence is weak, omit the memory.",
	}, " ")
	userPrompt := "Existing reflection memories for this session:\n" + emptyMarker(existing) +
		"\n\nNew transcript range:\n" + emptyMarker(transcript)
	req := hivy.CompletionRequest{
		Model: modelID,
		Messages: []hivy.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens: sessionReflectionMaxTokens,
	}
	resp, err := client.ChatCompletion(ctx, req)
	if err != nil {
		return sessionReflectionResult{}, err
	}
	result, err := parseSessionReflectionResponse(resp.Message.Content)
	if err != nil {
		return sessionReflectionResult{}, err
	}
	return result, nil
}

func parseSessionReflectionResponse(raw string) (sessionReflectionResult, error) {
	var result sessionReflectionResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &result); err != nil {
		return sessionReflectionResult{}, fmt.Errorf("decode reflection json: %w", err)
	}
	clean := make([]reflectionMemoryCandidate, 0, len(result.Memories))
	for _, candidate := range result.Memories {
		normalized, ok := normalizeReflectionCandidate(candidate)
		if ok {
			clean = append(clean, normalized)
		}
	}
	result.Memories = clean
	return result, nil
}

func normalizeReflectionCandidate(candidate reflectionMemoryCandidate) (reflectionMemoryCandidate, bool) {
	candidate.Content = strings.TrimSpace(candidate.Content)
	if candidate.Content == "" || unsafeReflectionContent(candidate.Content) {
		return reflectionMemoryCandidate{}, false
	}
	candidate.Scope = strings.TrimSpace(candidate.Scope)
	if candidate.Scope != "org" && candidate.Scope != "user" {
		return reflectionMemoryCandidate{}, false
	}
	candidate.Visibility = strings.TrimSpace(candidate.Visibility)
	if candidate.Visibility == "" {
		candidate.Visibility = "all_agents"
	}
	if candidate.Visibility != "all_agents" && candidate.Visibility != "this_agent" {
		return reflectionMemoryCandidate{}, false
	}
	candidate.Kind = strings.TrimSpace(candidate.Kind)
	if !validReflectionKind(candidate.Kind) {
		return reflectionMemoryCandidate{}, false
	}
	if candidate.Confidence < 0 || candidate.Confidence > 1 {
		return reflectionMemoryCandidate{}, false
	}
	candidate.ActorDisplayName = strings.TrimSpace(candidate.ActorDisplayName)
	candidate.ActorExternalRef = strings.TrimSpace(candidate.ActorExternalRef)
	candidate.SourceEventIDs = nonemptyStrings(candidate.SourceEventIDs)
	return candidate, true
}

func validReflectionKind(kind string) bool {
	switch kind {
	case "identity", "preference", "role", "project", "system", "environment", "workaround", "constraint", "decision", "integration", "other":
		return true
	default:
		return false
	}
}

func unsafeReflectionContent(content string) bool {
	value := strings.ToLower(content)
	for _, marker := range []string{"password", "private key", "api key", "secret key", "access token", "refresh token", "bearer ", "sk-"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func nonemptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func emptyMarker(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "(none)"
	}
	return value
}
