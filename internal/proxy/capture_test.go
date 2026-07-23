package proxy

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/observe"
)

func TestCaptureTransport_NonStreaming_OpenAI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"id":"chatcmpl-123","choices":[{"message":{"content":"hello"}}],"usage":{"prompt_tokens":100,"completion_tokens":50}}`)
	}))
	defer upstream.Close()

	captured := &observe.CapturedData{ProviderID: "openai"}
	ctx := observe.WithCapturedData(context.Background(), captured)

	req, _ := http.NewRequestWithContext(ctx, "POST", upstream.URL, nil)
	ct := &CaptureTransport{Inner: http.DefaultTransport}
	resp, err := ct.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "chatcmpl-123") {
		t.Error("response body should be intact")
	}

	if captured.Usage.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", captured.Usage.InputTokens)
	}
	if captured.Usage.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", captured.Usage.OutputTokens)
	}
	if captured.UpstreamStatus != 200 {
		t.Errorf("UpstreamStatus = %d, want 200", captured.UpstreamStatus)
	}
	if captured.IsStreaming {
		t.Error("should not be streaming")
	}
	if captured.TotalMs < 0 {
		t.Error("TotalMs should be non-negative")
	}
}

func TestCaptureTransport_NonStreaming_Anthropic(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"type":"message","usage":{"input_tokens":150,"output_tokens":80,"cache_read_input_tokens":40}}`)
	}))
	defer upstream.Close()

	captured := &observe.CapturedData{ProviderID: "anthropic"}
	ctx := observe.WithCapturedData(context.Background(), captured)
	req, _ := http.NewRequestWithContext(ctx, "POST", upstream.URL, nil)

	ct := &CaptureTransport{Inner: http.DefaultTransport}
	resp, err := ct.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	assertUsage(t, UsageData{
		InputTokens:  captured.Usage.InputTokens,
		OutputTokens: captured.Usage.OutputTokens,
		CachedTokens: captured.Usage.CachedTokens,
	}, 150, 80, 40, 0)
}

func TestCaptureTransport_Streaming_OpenAI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher := w.(http.Flusher)

		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		flusher.Flush()

		time.Sleep(10 * time.Millisecond)

		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":200,\"completion_tokens\":100,\"completion_tokens_details\":{\"reasoning_tokens\":15}}}\n\n")
		flusher.Flush()

		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	captured := &observe.CapturedData{ProviderID: "openai"}
	ctx := observe.WithCapturedData(context.Background(), captured)
	req, _ := http.NewRequestWithContext(ctx, "POST", upstream.URL, nil)

	ct := &CaptureTransport{Inner: http.DefaultTransport}
	resp, err := ct.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}

	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.Contains(string(body), "data: ") {
		t.Error("streaming body should contain SSE events")
	}

	if captured.IsStreaming != true {
		t.Error("should be streaming")
	}
	if captured.Usage.InputTokens != 200 {
		t.Errorf("InputTokens = %d, want 200", captured.Usage.InputTokens)
	}
	if captured.Usage.OutputTokens != 100 {
		t.Errorf("OutputTokens = %d, want 100", captured.Usage.OutputTokens)
	}
	if captured.Usage.ReasoningTokens != 15 {
		t.Errorf("ReasoningTokens = %d, want 15", captured.Usage.ReasoningTokens)
	}
	if captured.TTFBMs < 0 {
		t.Error("TTFBMs should be non-negative")
	}
	if captured.TotalMs < 0 {
		t.Error("TotalMs should be non-negative")
	}
	if captured.TotalMs < captured.TTFBMs {
		t.Error("TotalMs should be >= TTFBMs")
	}
}

func TestCaptureTransport_Streaming_XiaomiMiMoUsageSummary(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		// Sanitized from a live mimo-v2.5-pro stream. MiMo sends usage in a
		// separate final chunk whose choices array is empty.
		fmt.Fprint(w, "data: {\"id\":\"mimo-test\",\"choices\":[{\"delta\":{\"reasoning_content\":\"Compare decimals\"},\"finish_reason\":null,\"index\":0}],\"model\":\"mimo-v2.5-pro\",\"object\":\"chat.completion.chunk\"}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: {\"id\":\"mimo-test\",\"choices\":[],\"model\":\"mimo-v2.5-pro\",\"object\":\"chat.completion.chunk\",\"usage\":{\"completion_tokens\":128,\"prompt_tokens\":269,\"total_tokens\":397,\"completion_tokens_details\":{\"reasoning_tokens\":129},\"prompt_tokens_details\":{\"cached_tokens\":192}}}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	captured := &observe.CapturedData{ProviderID: "xiaomi"}
	ctx := observe.WithCapturedData(context.Background(), captured)
	req, _ := http.NewRequestWithContext(ctx, "POST", upstream.URL, nil)

	ct := &CaptureTransport{Inner: http.DefaultTransport}
	resp, err := ct.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()

	if captured.Usage.InputTokens != 269 {
		t.Errorf("InputTokens = %d, want 269", captured.Usage.InputTokens)
	}
	if captured.Usage.OutputTokens != 128 {
		t.Errorf("OutputTokens = %d, want 128", captured.Usage.OutputTokens)
	}
	if captured.Usage.CachedTokens != 192 {
		t.Errorf("CachedTokens = %d, want 192", captured.Usage.CachedTokens)
	}
	if captured.Usage.ReasoningTokens != 129 {
		t.Errorf("ReasoningTokens = %d, want 129", captured.Usage.ReasoningTokens)
	}
}

func TestCaptureTransport_Streaming_NovitaUsageSummary(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		// Sanitized from a live Novita Ling stream. Novita first emits zero
		// usage, followed by a separate final usage summary.
		fmt.Fprint(w, "data: {\"id\":\"novita-test\",\"choices\":[{\"delta\":{\"content\":\"NOVITA_OK\"}}],\"usage\":{\"prompt_tokens\":0,\"completion_tokens\":0,\"total_tokens\":0}}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: {\"id\":\"novita-test\",\"choices\":[],\"usage\":{\"prompt_tokens\":27,\"completion_tokens\":16,\"total_tokens\":43}}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	captured := &observe.CapturedData{ProviderID: "novita"}
	ctx := observe.WithCapturedData(context.Background(), captured)
	req, _ := http.NewRequestWithContext(ctx, "POST", upstream.URL, nil)

	ct := &CaptureTransport{Inner: http.DefaultTransport}
	resp, err := ct.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()

	if captured.Usage.InputTokens != 27 {
		t.Errorf("InputTokens = %d, want 27", captured.Usage.InputTokens)
	}
	if captured.Usage.OutputTokens != 16 {
		t.Errorf("OutputTokens = %d, want 16", captured.Usage.OutputTokens)
	}
}

func TestCaptureTransport_Streaming_EngyPreservesProviderCharge(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-sanitized\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"x_engy\":{\"request_id\":\"request-sanitized\",\"miner\":\"miner-sanitized\",\"charged_micro\":10}}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-sanitized\",\"choices\":[],\"usage\":{\"prompt_tokens\":15,\"completion_tokens\":32,\"total_tokens\":47,\"prompt_tokens_details\":{\"cached_tokens\":0}}}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	captured := &observe.CapturedData{ProviderID: "engy"}
	ctx := observe.WithCapturedData(context.Background(), captured)
	req, _ := http.NewRequestWithContext(ctx, "POST", upstream.URL, nil)

	ct := &CaptureTransport{Inner: http.DefaultTransport}
	resp, err := ct.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()

	if captured.Usage.InputTokens != 15 || captured.Usage.OutputTokens != 32 {
		t.Fatalf("usage = %#v", captured.Usage)
	}
	if math.Abs(captured.Usage.ProviderCostUSD-0.000010) > 1e-12 {
		t.Fatalf("ProviderCostUSD = %.12f, want 0.000010", captured.Usage.ProviderCostUSD)
	}
}

func TestCaptureTransport_Streaming_Anthropic(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		fmt.Fprint(w, "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":150,\"output_tokens\":0}}}\n\n")
		flusher.Flush()

		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"hello\"}}\n\n")
		flusher.Flush()

		fmt.Fprint(w, "data: {\"type\":\"message_delta\",\"usage\":{\"input_tokens\":150,\"output_tokens\":80,\"cache_read_input_tokens\":40}}\n\n")
		flusher.Flush()

		fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	captured := &observe.CapturedData{ProviderID: "anthropic"}
	ctx := observe.WithCapturedData(context.Background(), captured)
	req, _ := http.NewRequestWithContext(ctx, "POST", upstream.URL, nil)

	ct := &CaptureTransport{Inner: http.DefaultTransport}
	resp, err := ct.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()

	if captured.Usage.InputTokens != 150 {
		t.Errorf("InputTokens = %d, want 150", captured.Usage.InputTokens)
	}
	if captured.Usage.OutputTokens != 80 {
		t.Errorf("OutputTokens = %d, want 80", captured.Usage.OutputTokens)
	}
	if captured.Usage.CachedTokens != 40 {
		t.Errorf("CachedTokens = %d, want 40", captured.Usage.CachedTokens)
	}
}
