package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/usehivy/hivy/internal/model"
)

var (
	httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hivy_http_requests_total",
		Help: "Total HTTP requests handled by Hivy services.",
	}, []string{"service", "route", "method", "status_class"})
	httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "hivy_http_request_duration_seconds",
		Help:    "HTTP request duration by service and normalized route.",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "route", "method", "status_class"})
	httpInFlight = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hivy_http_requests_in_flight",
		Help: "HTTP requests currently being served.",
	}, []string{"service"})
	workflowOperations = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hivy_workflow_operations_total",
		Help: "Completed workflow operations by bounded domain, operation, and status.",
	}, []string{"domain", "operation", "status"})
	workflowDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "hivy_workflow_duration_seconds",
		Help:    "Workflow operation duration by bounded domain, operation, and status.",
		Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300},
	}, []string{"domain", "operation", "status"})
	llmGenerations = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hivy_llm_generations_total",
		Help: "LLM generations by provider, model, and outcome.",
	}, []string{"provider", "model", "status"})
	llmDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "hivy_llm_generation_duration_seconds",
		Help:    "End-to-end LLM generation duration.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 30, 60, 120, 300},
	}, []string{"provider", "model", "status"})
	llmTTFB = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "hivy_llm_generation_ttfb_seconds",
		Help:    "LLM time to first byte.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60},
	}, []string{"provider", "model", "status"})
	llmTokens = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hivy_llm_tokens_total",
		Help: "LLM tokens by provider, model, and token class.",
	}, []string{"provider", "model", "type"})
	llmCost = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hivy_llm_cost_usd_total",
		Help: "Recorded provider or registry-estimated LLM cost in USD.",
	}, []string{"provider", "model", "source"})
)

func init() {
	prometheus.MustRegister(
		httpRequests,
		httpDuration,
		httpInFlight,
		workflowOperations,
		workflowDuration,
		llmGenerations,
		llmDuration,
		llmTTFB,
		llmTokens,
		llmCost,
	)
}

// Handler exposes the process registry in Prometheus text format.
func Handler() http.Handler {
	return promhttp.Handler()
}

// HandlerWith exposes the process registry plus service-local collectors
// without mutating the global registry. This keeps collectors that own database
// handles scoped to the server instance that created them.
func HandlerWith(collectors ...prometheus.Collector) http.Handler {
	registry := prometheus.NewRegistry()
	for _, collector := range collectors {
		registry.MustRegister(collector)
	}
	return promhttp.HandlerFor(
		prometheus.Gatherers{prometheus.DefaultGatherer, registry},
		promhttp.HandlerOpts{},
	)
}

// TrackHTTPRequest increments the in-flight gauge and returns a completion
// callback. Route must be a normalized router pattern, never a raw path.
func TrackHTTPRequest(service string) func(route, method string, status int, duration time.Duration) {
	service = boundedLabel(service, "unknown")
	httpInFlight.WithLabelValues(service).Inc()
	return func(route, method string, status int, duration time.Duration) {
		httpInFlight.WithLabelValues(service).Dec()
		route = boundedLabel(route, "unmatched")
		method = boundedLabel(strings.ToUpper(method), "UNKNOWN")
		class := statusClass(status)
		httpRequests.WithLabelValues(service, route, method, class).Inc()
		httpDuration.WithLabelValues(service, route, method, class).Observe(duration.Seconds())
	}
}

// ObserveWorkflow records one bounded lifecycle operation.
func ObserveWorkflow(domain, operation, status string, duration time.Duration) {
	domain = boundedLabel(domain, "unknown")
	operation = boundedLabel(operation, "unknown")
	status = boundedLabel(status, "unknown")
	workflowOperations.WithLabelValues(domain, operation, status).Inc()
	workflowDuration.WithLabelValues(domain, operation, status).Observe(duration.Seconds())
}

// ObserveGeneration records aggregate LLM telemetry without customer IDs.
func ObserveGeneration(gen model.Generation) {
	provider := boundedLabel(gen.ProviderID, "unknown")
	modelID := boundedLabel(gen.Model, "unknown")
	status := "success"
	if gen.ErrorType != "" || gen.UpstreamStatus >= http.StatusBadRequest {
		status = "error"
	}
	llmGenerations.WithLabelValues(provider, modelID, status).Inc()
	llmDuration.WithLabelValues(provider, modelID, status).Observe(float64(gen.TotalMs) / 1000)
	if gen.TTFBMs != nil {
		llmTTFB.WithLabelValues(provider, modelID, status).Observe(float64(*gen.TTFBMs) / 1000)
	}
	addTokens(provider, modelID, "input", gen.InputTokens)
	addTokens(provider, modelID, "output", gen.OutputTokens)
	addTokens(provider, modelID, "cached", gen.CachedTokens)
	addTokens(provider, modelID, "reasoning", gen.ReasoningTokens)
	if gen.Cost > 0 {
		llmCost.WithLabelValues(provider, modelID, boundedLabel(gen.BillingCostSource, "unknown")).Add(gen.Cost)
	}
}

func addTokens(provider, modelID, tokenType string, count int) {
	if count > 0 {
		llmTokens.WithLabelValues(provider, modelID, tokenType).Add(float64(count))
	}
}

func statusClass(status int) string {
	if status < 100 || status > 599 {
		return "unknown"
	}
	return strconv.Itoa(status/100) + "xx"
}

func boundedLabel(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if len(value) > 160 {
		return value[:160]
	}
	return value
}
