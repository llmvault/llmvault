package metrics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/usehivy/hivy/internal/model"
)

func TestHTTPMiddlewareUsesNormalizedRoute(t *testing.T) {
	router := chi.NewRouter()
	router.Use(HTTPMiddleware("test-api"))
	router.Get("/v1/sessions/{sessionID}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/2632d7e3-bb21-4ffa-b0c3-f4f0fa42c44c", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if got := testutil.ToFloat64(httpRequests.WithLabelValues("test-api", "/v1/sessions/{sessionID}", http.MethodGet, "2xx")); got != 1 {
		t.Fatalf("request counter = %v, want 1", got)
	}
}

func TestObserveGenerationRecordsAggregateDimensions(t *testing.T) {
	ttfb := 250
	ObserveGeneration(model.Generation{
		ProviderID:        "openrouter",
		Model:             "openai/gpt-5",
		InputTokens:       10,
		OutputTokens:      5,
		CachedTokens:      2,
		ReasoningTokens:   3,
		Cost:              0.25,
		BillingCostSource: "provider",
		TTFBMs:            &ttfb,
		TotalMs:           int((2 * time.Second).Milliseconds()),
		UpstreamStatus:    http.StatusOK,
	})

	if got := testutil.ToFloat64(llmGenerations.WithLabelValues("openrouter", "openai/gpt-5", "success")); got != 1 {
		t.Fatalf("generation counter = %v, want 1", got)
	}
	if got := testutil.ToFloat64(llmTokens.WithLabelValues("openrouter", "openai/gpt-5", "input")); got != 10 {
		t.Fatalf("input tokens = %v, want 10", got)
	}
}

func TestAsynqMiddlewareRecordsErrorAndReleasesInFlight(t *testing.T) {
	task := asynq.NewTask("session:deliver", nil)
	wantErr := errors.New("delivery failed")
	handler := AsynqMiddleware()(asynq.HandlerFunc(func(context.Context, *asynq.Task) error {
		return wantErr
	}))
	before := testutil.ToFloat64(asynqTasks.WithLabelValues("unknown", task.Type(), "error"))

	err := handler.ProcessTask(t.Context(), task)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ProcessTask error = %v, want %v", err, wantErr)
	}
	if got := testutil.ToFloat64(asynqTasks.WithLabelValues("unknown", task.Type(), "error")); got != before+1 {
		t.Fatalf("task counter = %v, want %v", got, before+1)
	}
	if got := testutil.ToFloat64(asynqInFlight.WithLabelValues("unknown", task.Type())); got != 0 {
		t.Fatalf("in-flight gauge = %v, want 0", got)
	}
}
