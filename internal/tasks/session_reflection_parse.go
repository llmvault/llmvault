package tasks

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

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
	candidate.Kind = strings.TrimSpace(candidate.Kind)
	if !validReflectionKind(candidate.Kind) {
		return reflectionMemoryCandidate{}, false
	}
	if candidate.Confidence < sessionReflectionMinScore || candidate.Confidence > 1 {
		return reflectionMemoryCandidate{}, false
	}
	candidate.ExpiresAt = normalizeReflectionExpiry(candidate.ExpiresAt)
	candidate.Entities = nonemptyStrings(candidate.Entities)
	candidate.ActorDisplayName = strings.TrimSpace(candidate.ActorDisplayName)
	candidate.ActorExternalRef = strings.TrimSpace(candidate.ActorExternalRef)
	candidate.SourceEventIDs = nonemptyStrings(candidate.SourceEventIDs)
	return candidate, true
}

func validReflectionKind(kind string) bool {
	switch kind {
	case "preference", "rule", "decision", "convention", "org-fact", "person", "workaround", "commitment", "finding":
		return true
	default:
		return false
	}
}

// normalizeReflectionExpiry keeps expires_at only when it parses as an ISO
// date or RFC3339 timestamp; anything else becomes "" (indefinite).
func normalizeReflectionExpiry(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if _, err := time.Parse(layout, value); err == nil {
			return value
		}
	}
	return ""
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
