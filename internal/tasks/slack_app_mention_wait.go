package tasks

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
	"github.com/usehivy/hivy/internal/slackworkflow"
)

var errSlackNoFinalText = errors.New("slack app mention completed without final text")

func (h *SlackAppMentionHandler) waitQueuedSlackDelivery(ctx context.Context, eventID, queueID uuid.UUID) (*agentruntime.HTTPMessageResponse, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(4 * time.Minute)
	defer deadline.Stop()
	for {
		var queue model.SessionMessageQueue
		err := h.db.WithContext(ctx).First(&queue, "id = ?", queueID).Error
		if err != nil {
			return nil, fmt.Errorf("load slack queue delivery: %w", err)
		}
		if queue.DeliveredAt != nil && strings.TrimSpace(queue.RuntimeTurnID) != "" {
			_ = slackworkflow.RecordRuntimePosted(ctx, h.db, eventID, queue.RuntimeStreamID, queue.RuntimeTurnID)
			return &agentruntime.HTTPMessageResponse{
				StreamID: queue.RuntimeStreamID,
				TraceID:  queue.RuntimeTraceID,
				TurnID:   queue.RuntimeTurnID,
			}, nil
		}
		if queue.Status == "failed" {
			return nil, fmt.Errorf("queued slack delivery failed: %s", queue.LastError)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, fmt.Errorf("timed out waiting for queued slack delivery")
		case <-ticker.C:
		}
	}
}

func (h *SlackAppMentionHandler) runtimeClient(ctx context.Context, session model.Session, agent model.Agent) (*agentruntime.Client, error) {
	dispatcher := NewSessionMessageDeliverHandler(h.db, h.orchestrator, h.compileDeps, h.enqueuer)
	var latest model.Session
	if err := h.db.WithContext(ctx).First(&latest, "id = ?", session.ID).Error; err != nil {
		return nil, fmt.Errorf("load slack session for stream: %w", err)
	}
	_, client, err := dispatcher.ensureRuntimeClient(ctx, latest, &agent)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (h *SlackAppMentionHandler) waitForFinalText(ctx context.Context, client *agentruntime.Client, session model.Session, turnID string) (string, error) {
	if h.waitFinal != nil {
		return h.waitFinal(ctx, client, session, turnID)
	}
	path := "/sessions/" + url.PathEscape(session.ID.String()) + "/stream?from_turn_id=" + url.QueryEscape(turnID) + "&follow=true"
	resp, err := client.StreamHTTP(ctx, path)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	return readSlackFinalText(ctx, resp.Body)
}

func readSlackFinalText(ctx context.Context, body io.Reader) (string, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var eventType string
	var dataLines []string
	for scanner.Scan() {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
		case strings.HasPrefix(line, "data: "):
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		case line == "":
			text, terminal, err := slackStreamFrameText(eventType, strings.Join(dataLines, "\n"))
			if err != nil {
				return "", err
			}
			if strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text), nil
			}
			if terminal {
				return "", errSlackNoFinalText
			}
			eventType = ""
			dataLines = nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read slack runtime stream: %w", err)
	}
	return "", errSlackNoFinalText
}

func slackStreamFrameText(eventType, raw string) (string, bool, error) {
	payload := slackStreamPayload(raw)
	switch eventType {
	case runtimeevents.EventFinal:
		if !slackStreamPayloadFromMainAgent(payload) {
			return "", false, nil
		}
		return slackFinalText(payload), false, nil
	case runtimeevents.EventTurnCompleted, runtimeevents.EventDone:
		return "", slackStreamPayloadFromMainAgent(payload), nil
	case runtimeevents.EventTurnFailed, runtimeevents.EventError, runtimeevents.EventAgentError:
		if slackStreamPayloadFromMainAgent(payload) {
			return "", true, fmt.Errorf("runtime stream emitted %s", eventType)
		}
		return "", false, nil
	default:
		return "", false, nil
	}
}

func slackStreamPayload(raw string) map[string]any {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	return payload
}

func slackFinalText(payload map[string]any) string {
	for _, key := range []string{"text", "message", "content"} {
		if value, _ := payload[key].(string); strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func slackStreamPayloadFromMainAgent(payload map[string]any) bool {
	scope, ok := payload["scope"].(string)
	return ok && scope == "main"
}
