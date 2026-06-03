package gateway

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/usehivy/hivy/internal/precontext"
)

type recordingPreContextBuilder struct {
	mu       sync.Mutex
	requests []precontext.Request
}

func (b *recordingPreContextBuilder) Build(_ context.Context, req precontext.Request) ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.requests = append(b.requests, req)
	return []string{"## Recent sessions\n- cached context"}, nil
}

func (b *recordingPreContextBuilder) Count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.requests)
}

func TestServiceReceiveWebhookSendsPreContextOnlyForNewSession(t *testing.T) {
	db := connectGatewayTestDB(t)
	route := seedGatewayRoute(t, db)
	runtime := &recordingRuntime{}
	preload := &recordingPreContextBuilder{}
	service := NewService(db, runtime, nil, NewFakeSlackAdapter())
	service.SetPreContextBuilder(preload)

	if _, err := service.ReceiveWebhook(t.Context(), WebhookEnvelope{
		RouteID: route.ID,
		Body:    fakeSlackBody("400.000", "", "Start a new thread"),
	}); err != nil {
		t.Fatalf("receive first webhook: %v", err)
	}
	if _, err := service.ReceiveWebhook(t.Context(), WebhookEnvelope{
		RouteID: route.ID,
		Body:    fakeSlackBody("401.000", "400.000", "Continue the thread"),
	}); err != nil {
		t.Fatalf("receive second webhook: %v", err)
	}

	sent := runtime.Sent()
	if len(sent) != 2 {
		t.Fatalf("runtime sends = %d, want 2", len(sent))
	}
	if got := sent[0].DynamicContext; len(got) != 1 || !strings.Contains(got[0], "cached context") {
		t.Fatalf("first message missing pre-context: %#v", got)
	}
	if len(sent[1].DynamicContext) != 0 {
		t.Fatalf("existing thread should not include pre-context: %#v", sent[1].DynamicContext)
	}
	if preload.Count() != 1 {
		t.Fatalf("pre-context builder calls = %d, want 1", preload.Count())
	}
}
