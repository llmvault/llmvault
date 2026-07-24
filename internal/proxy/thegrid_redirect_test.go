package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/observe"
)

func TestCaptureTransportFollowsTheGridTemporaryRedirect(t *testing.T) {
	var calls int
	inner := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			if req.URL.String() != "https://api.thegrid.ai/v1/chat/completions" {
				t.Fatalf("first URL = %q", req.URL)
			}
			return &http.Response{
				StatusCode: http.StatusTemporaryRedirect,
				Header: http.Header{
					"Location": []string{"https://synapse.thegrid.ai/v1/chat/completions?token=signed"},
				},
				Body:    io.NopCloser(strings.NewReader("")),
				Request: req,
			}, nil
		case 2:
			if req.URL.Host != "synapse.thegrid.ai" {
				t.Fatalf("redirect host = %q", req.URL.Host)
			}
			if got := req.Header.Get("Authorization"); got != "Bearer test-key" {
				t.Fatalf("redirect Authorization = %q", got)
			}
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("read redirected body: %v", err)
			}
			if string(body) != `{"model":"text-standard"}` {
				t.Fatalf("redirected body = %q", body)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"usage":{"prompt_tokens":60,"completion_tokens":16,"estimated_cost":0.00000494}}`,
				)),
				Request: req,
			}, nil
		default:
			return nil, fmt.Errorf("unexpected request %d", calls)
		}
	})

	captured := &observe.CapturedData{ProviderID: "thegrid"}
	ctx := observe.WithCapturedData(context.Background(), captured)
	body := []byte(`{"model":"text-standard"}`)
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"https://api.thegrid.ai/v1/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-key")
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}

	transport := &CaptureTransport{Inner: inner}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if calls != 2 {
		t.Fatalf("upstream calls = %d, want 2", calls)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if captured.UpstreamStatus != http.StatusOK {
		t.Fatalf("captured upstream status = %d, want 200", captured.UpstreamStatus)
	}
	if captured.Usage.InputTokens != 60 || captured.Usage.OutputTokens != 16 {
		t.Fatalf("captured usage = %#v", captured.Usage)
	}
}

func TestCaptureTransportDoesNotFollowRedirectForOtherProviders(t *testing.T) {
	inner := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTemporaryRedirect,
			Header:     http.Header{"Location": []string{"https://example.com/redirect"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	})

	captured := &observe.CapturedData{ProviderID: "openai"}
	ctx := observe.WithCapturedData(context.Background(), captured)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", nil)

	resp, err := (&CaptureTransport{Inner: inner}).RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want 307", resp.StatusCode)
	}
}

func TestCaptureTransportRejectsUnexpectedTheGridRedirectHost(t *testing.T) {
	var calls int
	inner := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusTemporaryRedirect,
			Header:     http.Header{"Location": []string{"https://attacker.example/steal"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	})

	captured := &observe.CapturedData{ProviderID: "thegrid"}
	ctx := observe.WithCapturedData(context.Background(), captured)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.thegrid.ai/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer test-key")

	resp, err := (&CaptureTransport{Inner: inner}).RoundTrip(req)
	if err == nil || !strings.Contains(err.Error(), "destination is not allowed") {
		t.Fatalf("RoundTrip error = %v", err)
	}
	if resp != nil {
		closeResponseBody(resp)
		t.Fatalf("response = %#v, want nil", resp)
	}
	if calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", calls)
	}
}
