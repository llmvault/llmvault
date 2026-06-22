package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimestream"
)

const streamingE2EModel = "deepseek-v4-flash"

type streamingSessionCase struct {
	agent   agentSessionsAgentListItem
	channel agentSessionsChannel
	session agentSessionsMutation
	marker  string
	prompt  string
}

func TestAgentRuntimeRedisSequencingE2E(t *testing.T) {
	if os.Getenv("HIVY_AGENT_STREAMING_E2E") != "1" {
		t.Skip("set HIVY_AGENT_STREAMING_E2E=1 to run against the live compose stack")
	}
	loadEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Minute)
	defer cancel()

	apiBase := agentSessionsBaseURL("HIVY_API_BASE_URL", "HIVY_COMPOSE_API_PORT", "8080")
	workerBase := agentSessionsBaseURL("HIVY_WORKER_BASE_URL", "HIVY_COMPOSE_WORKER_HEALTH_PORT", "8090")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")
	requireAgentSessionsHealthy(t, ctx, workerBase, "worker")
	agentSessionsEnsureSystemOpenRouterCredential(t)

	db := agentSessionsOpenDB(t)
	redisClient := redis.NewClient(&redis.Options{Addr: testRedisAddrOrEnv()})
	t.Cleanup(func() { _ = redisClient.Close() })

	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	password := "agent-streaming-e2e-password"
	ownerEmail := "agent-streaming-owner-" + runID + "@example.com"
	ownerAuth := agentSessionsRegister(t, ctx, apiBase, ownerEmail, password, "Agent Streaming Owner "+runID)
	orgID := ownerAuth.Orgs[0].ID
	token := ownerAuth.AccessToken

	agents := make([]agentSessionsAgentListItem, 0, 2)
	for i := 0; i < 2; i++ {
		agent := agentStreamingCreateAgent(t, ctx, apiBase, token, orgID, fmt.Sprintf("Streaming %s %d", runID, i+1))
		agents = append(agents, agent)
	}

	var cases []streamingSessionCase
	for agentIndex, agent := range agents {
		channel := agentSessionsCreateChannel(t, ctx, apiBase, token, orgID, fmt.Sprintf("streaming-%s-%d", runID, agentIndex+1), agent.ID)
		for sessionIndex := 0; sessionIndex < 3; sessionIndex++ {
			marker := fmt.Sprintf("STREAMING_E2E_FINAL_%s_A%d_S%d", runID, agentIndex+1, sessionIndex+1)
			prompt := streamingE2EPrompt(runID, agentIndex+1, sessionIndex+1, marker)
			session := agentSessionsCreateSessionWithPayload(t, ctx, apiBase, token, orgID, map[string]any{
				"channel_id": channel.ID,
				"name":       fmt.Sprintf("Streaming %s A%d S%d", runID, agentIndex+1, sessionIndex+1),
			})
			if session.Session.ID == "" {
				t.Fatalf("session create returned empty id: %+v", session)
			}
			if session.Queued {
				t.Fatalf("empty session create queued a message: %+v", session)
			}
			if session.Event != nil {
				t.Fatalf("session create returned optimistic event=%+v, want nil until runtime commit", session.Event)
			}
			cases = append(cases, streamingSessionCase{agent: agent, channel: channel, session: session, marker: marker, prompt: prompt})
		}
	}

	ready := make(chan subscriberReady, len(cases)*3)
	results := make(chan subscriberResult, len(cases)*3)
	errs := make(chan error, len(cases)*3)
	var wg sync.WaitGroup
	for _, tc := range cases {
		for subscriberIndex := 0; subscriberIndex < 3; subscriberIndex++ {
			wg.Add(1)
			go func(tc streamingSessionCase, subscriberIndex int) {
				defer wg.Done()
				result, err := runGoSessionSubscriber(ctx, apiBase, token, orgID, tc.session.Session.ID, tc.marker, subscriberIndex, ready)
				if err != nil {
					errs <- err
					return
				}
				results <- result
			}(tc, subscriberIndex)
		}
	}
	waitForStreamingSubscribersReady(t, ctx, ready, errs, len(cases)*3)
	sendStreamingPrompts(t, ctx, apiBase, token, orgID, cases)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	bySession := map[string][]subscriberResult{}
	for result := range results {
		bySession[result.sessionID] = append(bySession[result.sessionID], result)
	}
	for _, tc := range cases {
		sessionID := tc.session.Session.ID
		sessionResults := bySession[sessionID]
		if len(sessionResults) != 3 {
			t.Fatalf("session %s subscriber results=%d, want 3", sessionID, len(sessionResults))
		}
		assertStreamingSubscribersConverged(t, sessionID, sessionResults)
		assertRuntimeRedisAndPostgresConverged(t, ctx, db, redisClient, sessionID)
	}

	t.Run("worker_resume", func(t *testing.T) {
		if os.Getenv("HIVY_AGENT_STREAMING_E2E_CHAOS") != "1" {
			t.Skip("set HIVY_AGENT_STREAMING_E2E_CHAOS=1 to run docker compose restart chaos checks")
		}
		runStreamingWorkerResumeChaos(t, ctx, apiBase, workerBase, token, orgID, cases[0].channel.ID, redisClient, db, runID)
	})

	t.Run("api_reconnect", func(t *testing.T) {
		if os.Getenv("HIVY_AGENT_STREAMING_E2E_CHAOS") != "1" {
			t.Skip("set HIVY_AGENT_STREAMING_E2E_CHAOS=1 to run docker compose restart chaos checks")
		}
		runStreamingAPIReconnectChaos(t, ctx, apiBase, workerBase, token, orgID, cases[1].channel.ID, redisClient, db, runID)
	})
}

func agentStreamingCreateAgent(t *testing.T, ctx context.Context, baseURL, token, orgID, name string) agentSessionsAgentListItem {
	t.Helper()
	var out agentSessionsAgentMutation
	agentSessionsJSON(t, ctx, http.MethodPost, baseURL+"/v1/agents", token, orgID, map[string]any{
		"name":                name,
		"description":         "Runtime Redis sequencing E2E agent",
		"instructions":        "Use tools exactly when requested. Keep responses deterministic and include requested markers exactly.",
		"model":               streamingE2EModel,
		"available_models":    []string{streamingE2EModel},
		"sandbox_strategy":    "per_session",
		"sandbox_tools":       []string{"bash"},
		"permissions":         map[string]any{"bash": true},
		"resources":           map[string]any{},
		"tools":               map[string]any{},
		"mcp_servers":         []any{},
		"skills":              map[string]any{},
		"sandbox_template_id": nil,
	}, http.StatusCreated, &out)
	if out.Agent.ID == "" {
		t.Fatalf("create streaming agent returned empty id: %+v", out)
	}
	return out.Agent
}

func streamingE2EPrompt(runID string, agentIndex, sessionIndex int, marker string) string {
	toolMarker := fmt.Sprintf("STREAMING_E2E_TOOL_%s_A%d_S%d", runID, agentIndex, sessionIndex)
	lines := make([]string, 0, 90)
	lines = append(lines,
		"This is the runtime Redis sequencing flagship E2E.",
		fmt.Sprintf("Before replying, call bash exactly once with this command: python3 -c 'print(%q)'.", toolMarker),
		"After the bash result, produce a numbered response with 90 short lines.",
		"Every line must contain the exact session marker "+marker+".",
		"Do not use markdown tables.",
	)
	return strings.Join(lines, "\n")
}

type goSessionSSEEvent struct {
	Name    string
	ID      string
	RawData string
	Payload map[string]any
}

type subscriberResult struct {
	sessionID string
	index     int
	committed []int64
	events    []goSessionSSEEvent
}

type subscriberReady struct {
	sessionID string
	index     int
}

func runGoSessionSubscriber(ctx context.Context, apiBase, token, orgID, sessionID, marker string, index int, ready chan<- subscriberReady) (subscriberResult, error) {
	streamCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	endpoint := fmt.Sprintf("%s/v1/sessions/%s/stream?after_seq=0", strings.TrimRight(apiBase, "/"), sessionID)
	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return subscriberResult{}, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-ID", orgID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return subscriberResult{}, fmt.Errorf("subscriber %s/%d open stream: %w", sessionID, index, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return subscriberResult{}, fmt.Errorf("subscriber %s/%d stream status=%d body=%s", sessionID, index, resp.StatusCode, body)
	}
	if ready != nil {
		select {
		case ready <- subscriberReady{sessionID: sessionID, index: index}:
		case <-streamCtx.Done():
			return subscriberResult{}, streamCtx.Err()
		}
	}

	events, err := readGoSessionStreamUntil(streamCtx, resp.Body, marker)
	if err != nil {
		return subscriberResult{}, fmt.Errorf("subscriber %s/%d: %w", sessionID, index, err)
	}
	committed := make([]int64, 0, len(events))
	for _, event := range events {
		if event.Name != "session.event" {
			continue
		}
		seq := int64FromAny(event.Payload["sequence_number"])
		if len(committed) > 0 && seq <= committed[len(committed)-1] {
			return subscriberResult{}, fmt.Errorf("subscriber %s/%d sequence regression: prev=%d next=%d", sessionID, index, committed[len(committed)-1], seq)
		}
		committed = append(committed, seq)
	}
	return subscriberResult{sessionID: sessionID, index: index, committed: committed, events: events}, nil
}

func waitForStreamingSubscribersReady(t *testing.T, ctx context.Context, ready <-chan subscriberReady, errs <-chan error, want int) {
	t.Helper()
	seen := map[string]bool{}
	deadline := time.NewTimer(2 * time.Minute)
	defer deadline.Stop()
	for len(seen) < want {
		select {
		case r := <-ready:
			seen[fmt.Sprintf("%s/%d", r.sessionID, r.index)] = true
		case err := <-errs:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatalf("context expired waiting for subscribers: %v", ctx.Err())
		case <-deadline.C:
			t.Fatalf("timed out waiting for subscribers ready: got=%d want=%d", len(seen), want)
		}
	}
}

func sendStreamingPrompts(t *testing.T, ctx context.Context, apiBase, token, orgID string, cases []streamingSessionCase) {
	t.Helper()
	errs := make(chan error, len(cases))
	var wg sync.WaitGroup
	for _, tc := range cases {
		tc := tc
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := agentSessionsSendMessage(t, ctx, apiBase, token, orgID, tc.session.Session.ID, tc.prompt)
			if out.Event != nil {
				errs <- fmt.Errorf("session %s send returned optimistic event=%+v, want nil until runtime commit", tc.session.Session.ID, out.Event)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func readGoSessionStreamUntil(ctx context.Context, body io.Reader, marker string) ([]goSessionSSEEvent, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024), 4*1024*1024)
	var events []goSessionSSEEvent
	var name string
	var id string
	var dataLines []string
	finalSeen := false
	terminalSeen := false
	flush := func() (bool, error) {
		if name == "" && id == "" && len(dataLines) == 0 {
			return false, nil
		}
		raw := strings.Join(dataLines, "\n")
		payload := map[string]any{}
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &payload); err != nil {
				payload = map[string]any{"_raw": raw}
			}
		}
		event := goSessionSSEEvent{Name: name, ID: id, RawData: raw, Payload: payload}
		events = append(events, event)
		if event.Name == "session.event" {
			eventType, _ := event.Payload["event_type"].(string)
			if eventType == "final" {
				finalSeen = true
			}
			if eventType == "turn_completed" || eventType == "turn_failed" || eventType == "done" {
				terminalSeen = true
			}
		}
		name = ""
		id = ""
		dataLines = nil
		return finalSeen && terminalSeen, nil
	}
	for {
		if ctx.Err() != nil {
			return events, ctx.Err()
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return events, err
			}
			_, err := flush()
			return events, err
		}
		line := scanner.Text()
		if line == "" {
			done, err := flush()
			if done || err != nil {
				return events, err
			}
			continue
		}
		if strings.HasPrefix(line, "event:") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "id:") {
			id = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
}

func assertStreamingSubscribersConverged(t *testing.T, sessionID string, results []subscriberResult) {
	t.Helper()
	sort.Slice(results, func(i, j int) bool { return results[i].index < results[j].index })
	want := sequenceSet(results[0].committed)
	if len(want) == 0 {
		t.Fatalf("session %s subscriber 0 saw no committed events", sessionID)
	}
	for _, result := range results {
		got := sequenceSet(result.committed)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("session %s subscriber %d committed set=%v want=%v", sessionID, result.index, got, want)
		}
	}
}

func sequenceSet(values []int64) []string {
	out := make([]string, 0, len(values))
	seen := map[int64]bool{}
	for _, value := range values {
		if !seen[value] {
			out = append(out, strconv.FormatInt(value, 10))
			seen[value] = true
		}
	}
	sort.Strings(out)
	return out
}

func assertRuntimeRedisAndPostgresConverged(t *testing.T, ctx context.Context, db *gorm.DB, redisClient *redis.Client, sessionID string) {
	t.Helper()
	events := waitForRuntimeRedisEvents(t, ctx, redisClient, sessionID)
	var previewCount, durableCount int
	durableBySeq := map[int64]runtimestream.Event{}
	lastSeq := int64(0)
	for _, event := range events {
		if event.RuntimeSeq <= lastSeq {
			t.Fatalf("session %s redis runtime_seq regression: prev=%d next=%d", sessionID, lastSeq, event.RuntimeSeq)
		}
		if event.RuntimeSeq != lastSeq+1 {
			t.Fatalf("session %s redis runtime_seq gap: prev=%d next=%d", sessionID, lastSeq, event.RuntimeSeq)
		}
		lastSeq = event.RuntimeSeq
		switch event.Durability {
		case runtimestream.DurabilityPreview:
			previewCount++
		case runtimestream.DurabilityDurable:
			durableCount++
			durableBySeq[event.RuntimeSeq] = event
		}
	}
	if previewCount == 0 || durableCount == 0 {
		t.Fatalf("session %s redis preview=%d durable=%d events=%d", sessionID, previewCount, durableCount, len(events))
	}

	rows := waitForSessionEventRows(t, ctx, db, sessionID, len(durableBySeq))
	if len(rows) >= len(events) {
		t.Fatalf("session %s postgres rows=%d redis raw events=%d; durable coalescing did not reduce writes", sessionID, len(rows), len(events))
	}
	seenSeq := map[int64]bool{}
	for _, row := range rows {
		if row.RuntimeSeq == nil {
			t.Fatalf("session %s row %s missing runtime_seq", sessionID, row.ID)
		}
		if row.SequenceNumber != *row.RuntimeSeq {
			t.Fatalf("session %s row %s sequence_number=%d runtime_seq=%d", sessionID, row.ID, row.SequenceNumber, *row.RuntimeSeq)
		}
		if seenSeq[row.SequenceNumber] {
			t.Fatalf("session %s duplicate sequence_number=%d", sessionID, row.SequenceNumber)
		}
		seenSeq[row.SequenceNumber] = true
		event, ok := durableBySeq[*row.RuntimeSeq]
		if !ok {
			t.Fatalf("session %s postgres runtime_seq=%d not found in durable redis events", sessionID, *row.RuntimeSeq)
		}
		if row.RuntimeEventID != event.EventID || row.EventType != event.EventType {
			t.Fatalf("session %s row seq=%d got event_id/type=%s/%s want=%s/%s", sessionID, row.SequenceNumber, row.RuntimeEventID, row.EventType, event.EventID, event.EventType)
		}
	}
}

func waitForRuntimeRedisEvents(t *testing.T, ctx context.Context, redisClient *redis.Client, sessionID string) []runtimestream.Event {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	var last []runtimestream.Event
	for time.Now().Before(deadline) {
		events := runtimeRedisEventsForSession(t, ctx, redisClient, sessionID)
		if hasRuntimeTerminal(events) {
			return events
		}
		last = events
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for redis runtime events: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for redis runtime terminal event; last=%d", len(last))
	return nil
}

func runtimeRedisEventsForSession(t *testing.T, ctx context.Context, redisClient *redis.Client, sessionID string) []runtimestream.Event {
	t.Helper()
	shard := runtimestream.ShardForSession(sessionID, runtimestream.DefaultShardCount)
	messages, err := redisClient.XRange(ctx, runtimestream.StreamKey(shard), "-", "+").Result()
	if err != nil {
		t.Fatalf("read redis stream shard=%d: %v", shard, err)
	}
	events := make([]runtimestream.Event, 0, len(messages))
	for _, message := range messages {
		event, err := runtimestream.EventFromStreamValues(message.Values)
		if err != nil {
			t.Fatalf("decode redis event %s: %v", message.ID, err)
		}
		if event.SessionID == sessionID {
			events = append(events, event)
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].RuntimeSeq < events[j].RuntimeSeq })
	return events
}

func hasRuntimeTerminal(events []runtimestream.Event) bool {
	for _, event := range events {
		if event.Durability != runtimestream.DurabilityDurable {
			continue
		}
		if event.EventType == "turn_completed" || event.EventType == "turn_failed" || event.EventType == "done" {
			return true
		}
	}
	return false
}

func waitForSessionEventRows(t *testing.T, ctx context.Context, db *gorm.DB, sessionID string, wantDurable int) []model.SessionEvent {
	t.Helper()
	sessionUUID := uuid.MustParse(sessionID)
	deadline := time.Now().Add(2 * time.Minute)
	var rows []model.SessionEvent
	for time.Now().Before(deadline) {
		rows = rows[:0]
		if err := db.WithContext(ctx).
			Where("session_id = ? AND runtime_seq IS NOT NULL", sessionUUID).
			Order("sequence_number ASC").
			Find(&rows).Error; err != nil {
			t.Fatalf("load session_events: %v", err)
		}
		if len(rows) >= wantDurable {
			return append([]model.SessionEvent(nil), rows...)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for session_events: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for durable rows: got=%d want=%d", len(rows), wantDurable)
	return nil
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	default:
		return 0
	}
}

func runStreamingWorkerResumeChaos(t *testing.T, ctx context.Context, apiBase, workerBase, token, orgID, channelID string, redisClient *redis.Client, db *gorm.DB, runID string) {
	t.Helper()
	session := agentSessionsCreateSession(t, ctx, apiBase, token, orgID, channelID, streamingE2EPrompt(runID, 9, 1, "STREAMING_E2E_WORKER_CHAOS_"+runID))
	restartComposeService(t, ctx, "worker")
	requireAgentSessionsHealthy(t, ctx, workerBase, "worker")
	result, err := runGoSessionSubscriber(ctx, apiBase, token, orgID, session.Session.ID, "STREAMING_E2E_WORKER_CHAOS_"+runID, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.committed) == 0 {
		t.Fatalf("worker chaos subscriber saw no committed events")
	}
	assertRuntimeRedisAndPostgresConverged(t, ctx, db, redisClient, session.Session.ID)
}

func runStreamingAPIReconnectChaos(t *testing.T, ctx context.Context, apiBase, workerBase, token, orgID, channelID string, redisClient *redis.Client, db *gorm.DB, runID string) {
	t.Helper()
	session := agentSessionsCreateSession(t, ctx, apiBase, token, orgID, channelID, streamingE2EPrompt(runID, 9, 2, "STREAMING_E2E_API_CHAOS_"+runID))
	restartComposeService(t, ctx, "api")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")
	requireAgentSessionsHealthy(t, ctx, workerBase, "worker")
	result, err := runGoSessionSubscriber(ctx, apiBase, token, orgID, session.Session.ID, "STREAMING_E2E_API_CHAOS_"+runID, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.committed) == 0 {
		t.Fatalf("api chaos subscriber saw no committed events")
	}
	assertRuntimeRedisAndPostgresConverged(t, ctx, db, redisClient, session.Session.ID)
}

func restartComposeService(t *testing.T, ctx context.Context, service string) {
	t.Helper()
	cmd := exec.CommandContext(ctx, "docker", "compose", "restart", service)
	cmd.Dir = ".."
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose restart %s: %v\n%s", service, err, output)
	}
}
