package proxy

import "net/http"

// FailoverTransport retries a request against the next catalog route before
// response headers reach the runtime. It never retries a successful response
// body, so streamed output cannot be duplicated or interleaved.
type FailoverTransport struct {
	Inner    http.RoundTripper
	Director *Director
	Router   *ModelRouter
}

func (t *FailoverTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	plan, ok := routePlanFromContext(req.Context())
	if !ok || t.Director == nil || t.Router == nil {
		return t.Inner.RoundTrip(req)
	}

	for {
		candidate := plan.candidates[plan.index]
		resp, err := t.Inner.RoundTrip(req)
		if !shouldFailover(resp, err) {
			if err == nil && resp != nil && resp.StatusCode < http.StatusBadRequest {
				t.Router.MarkSuccess(req.Context(), plan.canonicalModel, candidate)
			}
			return resp, err
		}

		t.Router.MarkFailure(req.Context(), plan.canonicalModel, candidate)
		if plan.index+1 >= len(plan.candidates) {
			return resp, err
		}
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if err := t.Director.applyCandidate(req, plan, plan.index+1); err != nil {
			return nil, err
		}
	}
}

func shouldFailover(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	if resp == nil {
		return false
	}
	return shouldFailoverStatus(resp.StatusCode)
}

func shouldFailoverStatus(status int) bool {
	switch status {
	case http.StatusPaymentRequired, http.StatusRequestTimeout, http.StatusTooManyRequests,
		http.StatusUnauthorized, http.StatusForbidden:
		return true
	default:
		return status >= http.StatusInternalServerError
	}
}
