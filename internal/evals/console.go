package evals

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

var evalSecretPattern = regexp.MustCompile(`(?i)(Bearer\s+)[A-Za-z0-9._~+/=-]+|ptok_[A-Za-z0-9._~+/=-]+|hvgw_[A-Za-z0-9._~+/=-]+|sk-[A-Za-z0-9._~+/=-]+`)

type ConsoleReporter struct {
	enabled         bool
	logger          *slog.Logger
	mu              sync.Mutex
	seenEvents      map[string]map[uuid.UUID]bool
	seenTasks       map[string]map[uuid.UUID]bool
	seenGenerations map[string]map[string]bool
}

func NewConsoleReporter(enabled bool, out io.Writer) *ConsoleReporter {
	if out == nil {
		out = os.Stdout
	}
	return &ConsoleReporter{
		enabled: enabled,
		logger: slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})),
		seenEvents:      map[string]map[uuid.UUID]bool{},
		seenTasks:       map[string]map[uuid.UUID]bool{},
		seenGenerations: map[string]map[string]bool{},
	}
}

func (r *ConsoleReporter) Enabled() bool {
	return r != nil && r.enabled
}

func (r *ConsoleReporter) Run(message string, attrs ...any) {
	if !r.Enabled() {
		return
	}
	r.logger.Info(message, attrs...)
}

func (r *ConsoleReporter) Trial(key TrialKey, message string, attrs ...any) {
	if !r.Enabled() {
		return
	}
	fields := []any{
		"suite", key.SuiteID,
		"model", key.Model,
		"case", key.CaseID,
		"run", key.RunIndex,
	}
	fields = append(fields, attrs...)
	r.logger.Info(message, fields...)
}

func (r *ConsoleReporter) ObserveEvidence(key TrialKey, ev Evidence) {
	if !r.Enabled() {
		return
	}
	for _, event := range ev.Events {
		if !r.markEvent(key, event.ID) {
			continue
		}
		r.Trial(key, "eval observed event",
			"event_type", event.EventType,
			"event_id", event.EventID,
			"db_event_id", event.ID.String(),
			"source", event.Source,
			"mode", event.Mode,
			"specialist", event.SpecialistSlug,
			"runtime_session_id", event.SessionID,
			"sequence", event.SequenceNumber,
			"event_at", event.EventAt,
			"payload", redactAndTrim(string(event.Payload), 1600),
		)
		if call, ok := toolCallFromPayload(event.Payload, event.EventAt); ok {
			r.Trial(key, "eval observed tool call",
				"tool", call.Name,
				"args", redactAndTrim(string(call.Args), 1000),
				"event_at", call.EventAt,
			)
		}
		if text := textFromPayload(event.Payload); strings.TrimSpace(text) != "" {
			r.Trial(key, "eval observed final text",
				"text", redactAndTrim(text, 1200),
				"event_at", event.EventAt,
			)
		}
	}
	for _, task := range ev.Tasks {
		if !r.markTask(key, task.ID) {
			continue
		}
		r.Trial(key, "eval observed specialist task",
			"task_id", task.ID.String(),
			"specialist", task.SpecialistSlug,
			"status", task.Status,
			"sandbox_id", task.SandboxID.String(),
			"created_at", task.CreatedAt,
			"brief", redactAndTrim(task.Brief, 1200),
		)
	}
}

func (r *ConsoleReporter) ObserveGenerations(key TrialKey, generations []model.Generation, judgeTokenJTI string) {
	if !r.Enabled() {
		return
	}
	for _, gen := range generations {
		if !r.markGeneration(key, gen.ID) {
			continue
		}
		r.Trial(key, "eval observed model generation",
			"generation_id", gen.ID,
			"model", gen.Model,
			"provider", gen.ProviderID,
			"path", gen.RequestPath,
			"streaming", gen.IsStreaming,
			"is_judge", strings.TrimSpace(judgeTokenJTI) != "" && gen.TokenJTI == judgeTokenJTI,
			"upstream_status", gen.UpstreamStatus,
			"input_tokens", gen.InputTokens,
			"output_tokens", gen.OutputTokens,
			"cached_tokens", gen.CachedTokens,
			"reasoning_tokens", gen.ReasoningTokens,
			"cost_usd", fmt.Sprintf("%.8f", gen.Cost),
			"credits", gen.CreditsDebited,
			"ttfb_ms", optionalInt(gen.TTFBMs),
			"total_ms", gen.TotalMs,
			"error_type", gen.ErrorType,
			"error", redactAndTrim(gen.ErrorMessage, 600),
			"tags", strings.Join(gen.Tags, ","),
			"created_at", gen.CreatedAt,
		)
	}
}

func (r *ConsoleReporter) markEvent(key TrialKey, id uuid.UUID) bool {
	if id == uuid.Nil {
		return true
	}
	return r.markUUID(r.seenEvents, key, id)
}

func (r *ConsoleReporter) markTask(key TrialKey, id uuid.UUID) bool {
	if id == uuid.Nil {
		return true
	}
	return r.markUUID(r.seenTasks, key, id)
}

func (r *ConsoleReporter) markGeneration(key TrialKey, id string) bool {
	if strings.TrimSpace(id) == "" {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	trialKey := key.String()
	seen := r.seenGenerations[trialKey]
	if seen == nil {
		seen = map[string]bool{}
		r.seenGenerations[trialKey] = seen
	}
	if seen[id] {
		return false
	}
	seen[id] = true
	return true
}

func (r *ConsoleReporter) markUUID(store map[string]map[uuid.UUID]bool, key TrialKey, id uuid.UUID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	trialKey := key.String()
	seen := store[trialKey]
	if seen == nil {
		seen = map[uuid.UUID]bool{}
		store[trialKey] = seen
	}
	if seen[id] {
		return false
	}
	seen[id] = true
	return true
}

func (k TrialKey) String() string {
	return fmt.Sprintf("%s:%s:%s:%d", k.SuiteID, k.Model, k.CaseID, k.RunIndex)
}

func redactAndTrim(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = evalSecretPattern.ReplaceAllStringFunc(value, func(match string) string {
		if strings.HasPrefix(strings.ToLower(match), "bearer ") {
			return "Bearer [REDACTED]"
		}
		return "[REDACTED]"
	})
	value = compactJSON(value)
	if limit > 0 && len(value) > limit {
		return value[:limit] + "...[truncated]"
	}
	return value
}

func compactJSON(value string) string {
	var payload any
	if err := json.Unmarshal([]byte(value), &payload); err != nil {
		return strings.Join(strings.Fields(value), " ")
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		return strings.Join(strings.Fields(value), " ")
	}
	return string(bytes)
}

func optionalInt(value *int) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%d", *value)
}

func uuidPtrString(value *uuid.UUID) string {
	if value == nil {
		return ""
	}
	return value.String()
}
