package e2e

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type subscriberResult struct {
	sessionID string
	index     int
	committed []int64
	events    []runtimeSSEEvent
}

type subscriberReady struct {
	sessionID string
	index     int
}

func runSandboxSessionSubscriber(ctx context.Context, apiBase, token, orgID, sessionID, marker string, index int, ready chan<- subscriberReady) (subscriberResult, error) {
	streamCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	access, err := fetchAgentSessionsSandboxAccessClient(streamCtx, apiBase, token, orgID, sessionID)
	if err != nil {
		return subscriberResult{}, fmt.Errorf("subscriber %s/%d sandbox access: %w", sessionID, index, err)
	}
	if err := validateAgentSessionsSandboxStreamAccess(access, sessionID); err != nil {
		return subscriberResult{}, fmt.Errorf("subscriber %s/%d sandbox access: %w", sessionID, index, err)
	}
	resp, err := openSandboxSessionStreamClient(streamCtx, sessionID, access)
	if err != nil {
		return subscriberResult{}, fmt.Errorf("subscriber %s/%d open stream: %w", sessionID, index, err)
	}
	defer resp.Body.Close()
	if ready != nil {
		select {
		case ready <- subscriberReady{sessionID: sessionID, index: index}:
		case <-streamCtx.Done():
			return subscriberResult{}, streamCtx.Err()
		}
	}

	events, err := readSandboxSessionStreamUntil(streamCtx, resp.Body, marker)
	if err != nil {
		return subscriberResult{}, fmt.Errorf("subscriber %s/%d: %w", sessionID, index, err)
	}
	committed := make([]int64, 0, len(events))
	for _, event := range events {
		if !isDurableSandboxRuntimeEvent(event) {
			continue
		}
		seq := int64FromAny(event.Payload["runtime_seq"])
		if seq == 0 {
			seq = int64FromAny(event.Payload["sequence"])
		}
		if seq == 0 {
			return subscriberResult{}, fmt.Errorf("subscriber %s/%d durable event missing runtime sequence: name=%s payload=%v", sessionID, index, event.Name, event.Payload)
		}
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
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := agentSessionsSendMessage(t, ctx, apiBase, token, orgID, tc.session.Session.ID, tc.prompt)
			if err := validateAgentSessionsBackendOwnedMutationEvent(out.Event); err != nil {
				errs <- fmt.Errorf("session %s send backend event invalid: %w", tc.session.Session.ID, err)
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

func readSandboxSessionStreamUntil(ctx context.Context, body io.Reader, marker string) ([]runtimeSSEEvent, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024), 4*1024*1024)
	var events []runtimeSSEEvent
	var name string
	var id string
	var dataLines []string
	finalSeen := false
	markerSeen := marker == ""
	terminalSeen := false
	flush := func() (bool, error) {
		if name == "" && id == "" && len(dataLines) == 0 {
			return false, nil
		}
		raw := strings.Join(dataLines, "\n")
		event := agentSessionsRuntimeEventFromSandboxSSE(name, raw)
		events = append(events, event)
		if event.Name == "final" {
			finalSeen = true
		}
		if marker != "" && strings.Contains(event.RawData, marker) {
			markerSeen = true
		}
		if event.Name == "turn_completed" || event.Name == "turn_failed" || event.Name == "done" {
			terminalSeen = true
		}
		name = ""
		id = ""
		dataLines = nil
		return finalSeen && markerSeen && terminalSeen, nil
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

func isDurableSandboxRuntimeEvent(event runtimeSSEEvent) bool {
	durability, _ := event.Payload["durability"].(string)
	return durability == "durable" && event.Name != "done"
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
