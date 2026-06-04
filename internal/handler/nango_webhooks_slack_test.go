package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

type recordingEnqueuer struct {
	tasks []*asynq.Task
}

func (e *recordingEnqueuer) Enqueue(task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	e.tasks = append(e.tasks, task)
	return &asynq.TaskInfo{}, nil
}

func (e *recordingEnqueuer) EnqueueContext(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	e.tasks = append(e.tasks, task)
	return &asynq.TaskInfo{}, nil
}

func (e *recordingEnqueuer) Close() error {
	return nil
}

func TestIsSlackProvider(t *testing.T) {
	tests := []struct {
		name string
		conn *model.Connection
		want bool
	}{
		{
			name: "slack connection",
			conn: &model.Connection{
				Integration: model.Integration{Provider: "slack"},
			},
			want: true,
		},
		{
			name: "github connection",
			conn: &model.Connection{
				Integration: model.Integration{Provider: "github"},
			},
			want: false,
		},
		{
			name: "nil connection",
			conn: nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSlackProvider(tt.conn)
			if got != tt.want {
				t.Errorf("isSlackProvider() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizedHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    http.Header
		expected map[string]string
	}{
		{
			name:     "empty headers",
			input:    http.Header{},
			expected: map[string]string{},
		},
		{
			name:     "single header",
			input:    http.Header{"Content-Type": {"application/json"}},
			expected: map[string]string{"content-type": "application/json"},
		},
		{
			name:     "multiple headers",
			input:    http.Header{"Content-Type": {"application/json"}, "X-Custom": {"value"}},
			expected: map[string]string{"content-type": "application/json", "x-custom": "value"},
		},
		{
			name:     "header case normalization",
			input:    http.Header{"CONTENT-TYPE": {"application/json"}},
			expected: map[string]string{"content-type": "application/json"},
		},
		{
			name:     "skip empty values",
			input:    http.Header{"Content-Type": {}},
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizedHeaders(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("normalizedHeaders() returned %d headers, want %d", len(got), len(tt.expected))
				return
			}
			for k, v := range tt.expected {
				if got[k] != v {
					t.Errorf("normalizedHeaders()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestGatewaySlackPayloadIncludesRuntimeAPIKey(t *testing.T) {
	connectionID := uuid.New()
	orgID := uuid.New()
	employeeID := uuid.New()
	sessionID := uuid.New()

	envelope := gateway.WebhookEnvelope{
		ConnectionID: connectionID,
		OrgID:        orgID,
		EmployeeID:   employeeID,
	}
	result := &gateway.ReceiveConnectionResult{
		Inbound: gateway.InboundEnvelope{
			ChannelID: "C123",
			ThreadID:  "1710000000.123",
			SenderID:  "U123",
			Raw:       map[string]any{"team_id": "T123"},
		},
		Session: model.EmployeeSession{
			ID: sessionID,
		},
		RuntimeConversationID: "gateway-conversation",
		RuntimeAPIKey:         "runtime-secret",
		RuntimeURL:            "https://runtime.example.com",
		StreamURL:             "https://runtime.example.com/gateway/http/streams/stream-123",
		ResponseStreamURL:     "https://runtime.example.com/gateway/http/response-streams/response-stream-123",
		TraceID:               "trace-123",
		TurnID:                "turn-123",
		ActionToken:           "action-token",
	}
	conn := &model.Connection{NangoConnectionID: "nango-conn"}

	payload := gatewaySlackPayload(envelope, result, conn, "slack")

	if payload.RuntimeAPIKey != result.RuntimeAPIKey {
		t.Fatalf("RuntimeAPIKey = %q, want %q", payload.RuntimeAPIKey, result.RuntimeAPIKey)
	}
	if payload.StreamURL != result.ResponseStreamURL {
		t.Fatalf("StreamURL = %q, want response stream %q", payload.StreamURL, result.ResponseStreamURL)
	}
	if payload.NangoConnID != conn.NangoConnectionID {
		t.Fatalf("NangoConnID = %q, want %q", payload.NangoConnID, conn.NangoConnectionID)
	}
	if payload.TeamID != "T123" {
		t.Fatalf("TeamID = %q, want T123", payload.TeamID)
	}
}

func TestGatewaySlackPayloadFallsBackToFullStreamURL(t *testing.T) {
	envelope := gateway.WebhookEnvelope{
		ConnectionID: uuid.New(),
		OrgID:        uuid.New(),
		EmployeeID:   uuid.New(),
	}
	result := &gateway.ReceiveConnectionResult{
		Inbound: gateway.InboundEnvelope{
			ChannelID: "C123",
			ThreadID:  "1710000000.123",
		},
		Session:   model.EmployeeSession{ID: uuid.New()},
		StreamURL: "https://runtime.example.com/gateway/http/streams/stream-123",
	}

	payload := gatewaySlackPayload(envelope, result, &model.Connection{}, "slack")

	if payload.StreamURL != result.StreamURL {
		t.Fatalf("StreamURL = %q, want fallback %q", payload.StreamURL, result.StreamURL)
	}
}

func TestEnqueueGatewaySlackStatusBuildsEarlyStatusTask(t *testing.T) {
	enq := &recordingEnqueuer{}
	handler := &NangoWebhookHandler{enqueuer: enq}
	eventID := uuid.New()

	handler.enqueueGatewaySlackStatus(context.Background(), gateway.ConnectionInboundAccepted{
		Envelope: gateway.WebhookEnvelope{
			ConnectionID: uuid.New(),
			OrgID:        uuid.New(),
			EmployeeID:   uuid.New(),
			Provider:     gateway.SlackProvider,
			ProviderKey:  "slack",
			NangoConnID:  "nango-conn",
		},
		Inbound: gateway.InboundEnvelope{
			ChannelID: "C123",
			ThreadID:  "1710000000.123",
			Raw:       map[string]any{"team_id": "T123"},
		},
		Event: model.EmployeeGatewayEvent{ID: eventID},
	})

	if len(enq.tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(enq.tasks))
	}
	if enq.tasks[0].Type() != tasks.TypeGatewaySlackStatus {
		t.Fatalf("task type = %q, want %q", enq.tasks[0].Type(), tasks.TypeGatewaySlackStatus)
	}
	var payload tasks.GatewaySlackStatusPayload
	if err := json.Unmarshal(enq.tasks[0].Payload(), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ChannelID != "C123" || payload.ThreadTS != "1710000000.123" {
		t.Fatalf("payload thread fields = %#v", payload)
	}
	if payload.TeamID != "T123" || payload.NangoConnID != "nango-conn" || payload.ProviderKey != "slack" {
		t.Fatalf("payload integration fields = %#v", payload)
	}
	if payload.EventID != eventID.String() {
		t.Fatalf("EventID = %q, want %q", payload.EventID, eventID)
	}
}

func TestIsSlackGatewayEvent(t *testing.T) {
	if !isSlackGatewayEvent(map[string]any{"provider": "slack"}) {
		t.Fatal("expected slack gateway event")
	}
	if isSlackGatewayEvent(map[string]any{"provider": "fake-slack"}) {
		t.Fatal("fake-slack must still use the generic gateway final path")
	}
	if isSlackGatewayEvent(map[string]any{}) {
		t.Fatal("missing provider must still use the generic gateway final path")
	}
}
