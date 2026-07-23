package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
)

const maxFailoverStreamProbeBytes = 1 << 20

var errStreamEndedBeforeEvent = errors.New("upstream stream ended before its first SSE event")
var errStreamReportedProviderError = errors.New("upstream reported an error before producing model output")

// FailoverTransport retries a request against the next catalog route before
// response headers reach the runtime. For SSE responses, it can also retry while
// probing for the first complete data event. It never switches routes after
// model output has been released, avoiding duplicated or interleaved output.
type FailoverTransport struct {
	Inner    http.RoundTripper
	Director *Director
	Router   *ModelRouter

	applyCandidate func(*http.Request, *routePlan, int) error
}

func (t *FailoverTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	plan, ok := routePlanFromContext(req.Context())
	if !ok || (t.Director == nil && t.applyCandidate == nil) || t.Router == nil {
		return t.Inner.RoundTrip(req)
	}

	for {
		candidate := plan.candidates[plan.index]
		resp, err := t.Inner.RoundTrip(req)
		if !shouldFailover(resp, err) {
			if plan.streaming && resp != nil && resp.Body != nil && plan.index+1 < len(plan.candidates) {
				resp.Body = &failoverStreamBody{
					transport: t,
					req:       req,
					plan:      plan,
					candidate: candidate,
					body:      resp.Body,
				}
				return resp, nil
			}
			if err == nil && resp != nil && isSuccessfulStatus(resp.StatusCode) {
				t.Router.MarkSuccess(req.Context(), plan.canonicalModel, candidate)
			}
			return resp, err
		}

		if shouldMarkRouteFailure(resp, err) {
			t.Router.MarkFailure(req.Context(), plan.canonicalModel, candidate)
		}
		if plan.index+1 >= len(plan.candidates) {
			return resp, err
		}
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if err := t.advance(req, plan, plan.index+1); err != nil {
			return nil, err
		}
	}
}

func (t *FailoverTransport) advance(req *http.Request, plan *routePlan, index int) error {
	if t.applyCandidate != nil {
		return t.applyCandidate(req, plan, index)
	}
	return t.Director.applyCandidate(req, plan, index)
}

func shouldFailover(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	if resp == nil {
		return true
	}
	if resp.Body == nil {
		return true
	}
	return shouldFailoverStatus(resp.StatusCode)
}

func shouldFailoverStatus(status int) bool {
	return !isSuccessfulStatus(status)
}

func isSuccessfulStatus(status int) bool {
	return status >= http.StatusOK && status < http.StatusMultipleChoices
}

// Only provider-availability failures enter the shared cooldown. Request-specific
// 4xx responses still try the next route for this request without allowing one
// malformed prompt to blacklist a provider for every session.
func shouldMarkRouteFailure(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	if resp == nil {
		return true
	}
	if resp.Body == nil {
		return true
	}
	status := resp.StatusCode
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect,
		http.StatusPaymentRequired, http.StatusRequestTimeout, http.StatusTooManyRequests,
		http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		return true
	default:
		return status >= http.StatusInternalServerError
	}
}

// failoverStreamBody delays the first downstream bytes until a complete SSE
// event is available. Until then, switching routes is safe because the caller
// has observed no model output. Once an event is released, subsequent body
// failures are returned as-is so output is never duplicated or interleaved.
type failoverStreamBody struct {
	transport *FailoverTransport
	req       *http.Request
	plan      *routePlan
	candidate RouteCandidate
	body      io.ReadCloser

	prepareOnce sync.Once
	prepareErr  error
}

func (b *failoverStreamBody) Read(p []byte) (int, error) {
	b.prepareOnce.Do(b.prepare)
	if b.prepareErr != nil {
		return 0, b.prepareErr
	}
	return b.body.Read(p)
}

func (b *failoverStreamBody) Close() error {
	if b.body == nil {
		return nil
	}
	return b.body.Close()
}

func (b *failoverStreamBody) prepare() {
	for {
		probed, err := probeFirstSSEEvent(b.body)
		if err == nil {
			b.body = probed
			b.transport.Router.MarkSuccess(b.req.Context(), b.plan.canonicalModel, b.candidate)
			return
		}

		b.transport.Router.MarkFailure(b.req.Context(), b.plan.canonicalModel, b.candidate)
		_ = b.body.Close()
		if b.plan.index+1 >= len(b.plan.candidates) {
			b.prepareErr = err
			return
		}
		if err := b.transport.advance(b.req, b.plan, b.plan.index+1); err != nil {
			b.prepareErr = err
			return
		}

		b.candidate = b.plan.candidates[b.plan.index]
		resp, roundTripErr := b.transport.Inner.RoundTrip(b.req)
		if shouldFailover(resp, roundTripErr) {
			if shouldMarkRouteFailure(resp, roundTripErr) {
				b.transport.Router.MarkFailure(b.req.Context(), b.plan.canonicalModel, b.candidate)
			}
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			if b.plan.index+1 >= len(b.plan.candidates) {
				if roundTripErr != nil {
					b.prepareErr = roundTripErr
				} else if resp != nil {
					b.prepareErr = fmt.Errorf("fallback model endpoint returned HTTP %d", resp.StatusCode)
				} else {
					b.prepareErr = errors.New("fallback model endpoint returned no response")
				}
				return
			}
			if err := b.transport.advance(b.req, b.plan, b.plan.index+1); err != nil {
				b.prepareErr = err
				return
			}
			b.candidate = b.plan.candidates[b.plan.index]
			continue
		}
		if resp == nil || resp.Body == nil {
			b.prepareErr = errors.New("fallback model endpoint returned an empty response")
			return
		}
		b.body = resp.Body
	}
}

func probeFirstSSEEvent(body io.ReadCloser) (io.ReadCloser, error) {
	var buffered bytes.Buffer
	chunk := make([]byte, 32*1024)
	for buffered.Len() <= maxFailoverStreamProbeBytes {
		n, err := body.Read(chunk)
		if n > 0 {
			_, _ = buffered.Write(chunk[:n])
			payload, eventError, complete := firstCompleteSSEDataEvent(buffered.Bytes())
			if complete && (eventError || sseDataEventIsError(payload)) {
				return nil, errStreamReportedProviderError
			}
			if complete {
				return &replayReadCloser{
					prefix:      bytes.NewReader(buffered.Bytes()),
					inner:       body,
					terminalErr: err,
				}, nil
			}
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errStreamEndedBeforeEvent, err)
		}
	}
	return nil, fmt.Errorf("%w: first event exceeded %d bytes", errStreamEndedBeforeEvent, maxFailoverStreamProbeBytes)
}

func firstCompleteSSEDataEvent(data []byte) ([]byte, bool, bool) {
	normalized := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	blocks := bytes.Split(normalized, []byte("\n\n"))
	for _, block := range blocks[:len(blocks)-1] {
		var payload bytes.Buffer
		eventError := false
		for _, line := range bytes.Split(block, []byte("\n")) {
			switch {
			case bytes.HasPrefix(line, []byte("event:")):
				eventError = bytes.Equal(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("event:"))), []byte("error"))
			case bytes.HasPrefix(line, []byte("data:")):
				if payload.Len() > 0 {
					_ = payload.WriteByte('\n')
				}
				_, _ = payload.Write(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:"))))
			}
		}
		if payload.Len() > 0 {
			return payload.Bytes(), eventError, true
		}
	}
	return nil, false, false
}

func sseDataEventIsError(payload []byte) bool {
	if bytes.Equal(payload, []byte("[DONE]")) {
		return false
	}
	var event struct {
		Error json.RawMessage `json:"error"`
		Type  string          `json:"type"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return false
	}
	return (len(event.Error) > 0 && !bytes.Equal(bytes.TrimSpace(event.Error), []byte("null"))) ||
		event.Type == "error"
}

type replayReadCloser struct {
	prefix      *bytes.Reader
	inner       io.ReadCloser
	terminalErr error
}

func (r *replayReadCloser) Read(p []byte) (int, error) {
	if r.prefix.Len() > 0 {
		return r.prefix.Read(p)
	}
	if r.terminalErr != nil {
		err := r.terminalErr
		r.terminalErr = nil
		return 0, err
	}
	return r.inner.Read(p)
}

func (r *replayReadCloser) Close() error {
	return r.inner.Close()
}
