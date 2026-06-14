package tasks

import (
	"regexp"
	"sort"
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

var memorySecretPattern = regexp.MustCompile(`(?i)(ptok_|xox[baprs]-|sk-[a-z0-9]|api[_-]?key|secret|token|password)\s*[:=]\s*\S+`)

func sessionEventsContainSecret(events []model.SessionEvent) bool {
	for _, event := range events {
		payload := agentMemoryPayload(event)
		for _, key := range []string{"text", "message", "result_summary"} {
			if value := firstPayloadString(payload, key); value != "" && memorySecretPattern.MatchString(value) {
				return true
			}
		}
	}
	return false
}

func shouldIncludeAgentMemoryMessage(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" || memorySecretPattern.MatchString(text) {
		return false
	}
	return true
}

func meaningfulAgentMemoryTranscript(transcript string, events []model.SessionEvent) bool {
	transcript = strings.TrimSpace(transcript)
	if transcript == "" || memorySecretPattern.MatchString(transcript) {
		return false
	}
	hasUser := false
	hasCheckpoint := false
	for _, event := range events {
		if event.EventType == "user.message.received" {
			hasUser = true
		}
		if event.EventType == "final" || event.EventType == "turn_completed" || event.EventType == "session.completed" {
			hasCheckpoint = true
		}
	}
	return hasUser && hasCheckpoint
}

func agentMemoryTags(agent *model.Agent, source string) []string {
	tags := []string{
		"company:" + agent.OrgID.String(),
		"source:" + sanitizeMemoryTagValue(source),
		"visibility:company",
		"memory_type:company_context",
	}
	return tags
}

func dominantAgentMemorySource(events []model.SessionEvent) string {
	counts := map[string]int{}
	for _, event := range events {
		source := strings.TrimSpace(event.Source)
		if source == "" {
			source = "manual"
		}
		counts[source]++
	}
	type pair struct {
		source string
		count  int
	}
	pairs := make([]pair, 0, len(counts))
	for source, count := range counts {
		pairs = append(pairs, pair{source: source, count: count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count == pairs[j].count {
			return pairs[i].source < pairs[j].source
		}
		return pairs[i].count > pairs[j].count
	})
	if len(pairs) == 0 {
		return "manual"
	}
	return pairs[0].source
}

func agentMemoryPayload(event model.SessionEvent) map[string]any {
	if event.Payload == nil {
		return map[string]any{}
	}
	return map[string]any(event.Payload)
}

func sanitizeMemoryTagValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "manual"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '.' || r == '/':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-_")
	if out == "" {
		return "manual"
	}
	return out
}
