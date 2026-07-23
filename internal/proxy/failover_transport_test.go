package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func TestShouldFailover(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   bool
	}{
		{name: "success", status: http.StatusOK, want: false},
		{name: "provider billing error", status: http.StatusPaymentRequired, want: true},
		{name: "rate limited", status: http.StatusTooManyRequests, want: true},
		{name: "provider unavailable", status: http.StatusServiceUnavailable, want: true},
		{name: "invalid client request can use another route", status: http.StatusBadRequest, want: true},
		{name: "model unavailable", status: http.StatusNotFound, want: true},
		{name: "unprocessable by provider", status: http.StatusUnprocessableEntity, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldFailoverStatus(tc.status); got != tc.want {
				t.Fatalf("shouldFailoverStatus() = %v, want %v", got, tc.want)
			}
		})
	}
	if !shouldFailover(nil, errors.New("dial failed")) {
		t.Fatal("transport error must fail over")
	}
	if !shouldFailover(nil, nil) {
		t.Fatal("missing upstream response must fail over")
	}
	if !shouldFailover(&http.Response{StatusCode: http.StatusOK}, nil) {
		t.Fatal("successful response without a body must fail over")
	}
	if shouldMarkRouteFailure(&http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader(`{"error":"invalid request"}`)),
	}, nil) {
		t.Fatal("request-specific 400 must not put the provider into shared cooldown")
	}
	if !shouldMarkRouteFailure(nil, errors.New("connection reset")) {
		t.Fatal("transport error must put the provider into shared cooldown")
	}
	if !shouldMarkRouteFailure(nil, nil) {
		t.Fatal("missing upstream response must put the provider into shared cooldown")
	}
}

func TestCandidatesForRoutesPreservesCatalogOrderAndSkipsMissingCredentials(t *testing.T) {
	atlasCloudID := uuid.New()
	routes := []registry.ModelRoute{
		{ProviderID: "atlascloud", ModelID: "deepseek-ai/deepseek-v4-flash"},
		{ProviderID: "provider-a", ModelID: "deepseek/a"},
		{ProviderID: "provider-b", ModelID: "deepseek/b"},
	}
	credentials := []model.Credential{
		{ID: atlasCloudID, ProviderID: "atlascloud"},
		{ID: uuid.New(), ProviderID: "provider-b"},
	}

	candidates := candidatesForRoutes("deepseek-v4", routes, credentials)
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(candidates))
	}
	if candidates[0].ProviderID != "atlascloud" ||
		candidates[0].UpstreamID != "deepseek-ai/deepseek-v4-flash" ||
		candidates[0].CanonicalModelID != "deepseek-v4" {
		t.Fatalf("primary candidate = %#v", candidates[0])
	}
	if candidates[1].ProviderID != "provider-b" ||
		candidates[1].UpstreamID != "deepseek/b" ||
		candidates[1].CanonicalModelID != "deepseek-v4" {
		t.Fatalf("fallback candidate = %#v", candidates[1])
	}
}

func TestCandidatesForRoutesPreservesFallbackCanonicalModel(t *testing.T) {
	novitaID := uuid.New()
	routes := []registry.ModelRoute{
		{ProviderID: "xiaomi", ModelID: "mimo-v2.5-pro-ultraspeed"},
		{
			ProviderID:       "novita",
			ModelID:          "xiaomimimo/mimo-v2.5-pro",
			CanonicalModelID: "mimo-v2.5-pro",
		},
	}
	credentials := []model.Credential{
		{ID: uuid.New(), ProviderID: "xiaomi"},
		{ID: novitaID, ProviderID: "novita"},
	}

	candidates := candidatesForRoutes("mimo-v2.5-pro-ultraspeed", routes, credentials)
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(candidates))
	}
	fallback := candidates[1]
	if fallback.CredentialID != novitaID.String() ||
		fallback.ProviderID != "novita" ||
		fallback.UpstreamID != "xiaomimimo/mimo-v2.5-pro" ||
		fallback.CanonicalModelID != "mimo-v2.5-pro" {
		t.Fatalf("fallback candidate = %#v", fallback)
	}
}

func TestFailoverTransportFallsBackWhenStreamEndsBeforeFirstEvent(t *testing.T) {
	plan := testStreamingRoutePlan()
	request := requestWithRoutePlan(plan)
	var calls int
	transport := testFailoverTransport(plan, func(*http.Request) (*http.Response, error) {
		calls++
		if plan.index == 0 {
			return sseResponse(&singleReadBody{err: io.ErrUnexpectedEOF}), nil
		}
		return sseResponse(io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))), nil
	})

	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read fallback stream: %v", err)
	}
	if calls != 2 {
		t.Fatalf("upstream calls = %d, want 2", calls)
	}
	if got := string(body); !strings.Contains(got, `"content":"ok"`) {
		t.Fatalf("fallback body = %q", got)
	}
}

func TestFailoverTransportFallsBackOnProviderBadRequest(t *testing.T) {
	plan := testStreamingRoutePlan()
	request := requestWithRoutePlan(plan)
	var calls int
	transport := testFailoverTransport(plan, func(*http.Request) (*http.Response, error) {
		calls++
		if plan.index == 0 {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"error":"unsupported model"}`)),
			}, nil
		}
		return sseResponse(io.NopCloser(strings.NewReader("data: [DONE]\n\n"))), nil
	})

	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read fallback response: %v", err)
	}
	if calls != 2 || string(body) != "data: [DONE]\n\n" {
		t.Fatalf("calls = %d body = %q", calls, body)
	}
	if plan.index != 1 {
		t.Fatalf("selected route index = %d, want 1", plan.index)
	}
}

func TestFailoverTransportFallsBackOnFirstSSEErrorEvent(t *testing.T) {
	plan := testStreamingRoutePlan()
	request := requestWithRoutePlan(plan)
	var calls int
	transport := testFailoverTransport(plan, func(*http.Request) (*http.Response, error) {
		calls++
		if plan.index == 0 {
			return sseResponse(io.NopCloser(strings.NewReader(
				"event: error\ndata: {\"error\":{\"message\":\"provider overloaded\"}}\n\n",
			))), nil
		}
		return sseResponse(io.NopCloser(strings.NewReader(
			"data: {\"choices\":[{\"delta\":{\"content\":\"fallback\"}}]}\n\n",
		))), nil
	})

	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read fallback stream: %v", err)
	}
	if calls != 2 {
		t.Fatalf("upstream calls = %d, want 2", calls)
	}
	if got := string(body); !strings.Contains(got, `"content":"fallback"`) {
		t.Fatalf("fallback body = %q", got)
	}
}

func TestFailoverTransportNeverSwitchesAfterFirstEvent(t *testing.T) {
	plan := testStreamingRoutePlan()
	request := requestWithRoutePlan(plan)
	var calls int
	transport := testFailoverTransport(plan, func(*http.Request) (*http.Response, error) {
		calls++
		return sseResponse(&singleReadBody{
			data: []byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"),
			err:  io.ErrUnexpectedEOF,
		}), nil
	})

	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("read error = %v, want unexpected EOF", err)
	}
	if calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", calls)
	}
	if !strings.Contains(string(body), `"content":"partial"`) {
		t.Fatalf("body = %q", body)
	}
}

func testStreamingRoutePlan() *routePlan {
	return &routePlan{
		canonicalModel: "mimo-v2.5-pro-ultraspeed",
		streaming:      true,
		candidates: []RouteCandidate{
			{
				CredentialID:     uuid.NewString(),
				ProviderID:       "xiaomi",
				UpstreamID:       "mimo-v2.5-pro-ultraspeed",
				CanonicalModelID: "mimo-v2.5-pro-ultraspeed",
			},
			{
				CredentialID:     uuid.NewString(),
				ProviderID:       "novita",
				UpstreamID:       "xiaomimimo/mimo-v2.5-pro",
				CanonicalModelID: "mimo-v2.5-pro",
			},
		},
	}
}

func requestWithRoutePlan(plan *routePlan) *http.Request {
	request, _ := http.NewRequest(http.MethodPost, "https://proxy.example/v1/chat/completions", nil)
	return request.WithContext(context.WithValue(request.Context(), routePlanContextKey{}, plan))
}

func testFailoverTransport(plan *routePlan, roundTrip func(*http.Request) (*http.Response, error)) *FailoverTransport {
	return &FailoverTransport{
		Inner:    roundTripperFunc(roundTrip),
		Director: &Director{},
		Router:   &ModelRouter{},
		applyCandidate: func(_ *http.Request, got *routePlan, index int) error {
			if got != plan {
				return errors.New("unexpected route plan")
			}
			got.index = index
			return nil
		},
	}
}

func sseResponse(body io.ReadCloser) *http.Response {
	header := make(http.Header)
	header.Set("Content-Type", "text/event-stream")
	return &http.Response{StatusCode: http.StatusOK, Header: header, Body: body}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type singleReadBody struct {
	data []byte
	err  error
	read bool
}

func (b *singleReadBody) Read(p []byte) (int, error) {
	if b.read {
		return 0, io.EOF
	}
	b.read = true
	return copy(p, b.data), b.err
}

func (*singleReadBody) Close() error {
	return nil
}
