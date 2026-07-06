package linear

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// ---- fixtureSource: a minimal interfaces.Source with Nango fields ----

// fixtureSource implements interfaces.Source plus the connection methods
// the Build factory probes for. SourceKind is the literal "linear".
type fixtureSource struct {
	cfg json.RawMessage
}

func newFixtureSource(config string) *fixtureSource {
	return &fixtureSource{cfg: json.RawMessage(config)}
}

func (s *fixtureSource) SourceID() string               { return "src-linear-fixture" }
func (s *fixtureSource) OrgID() string                  { return "org-linear-fixture" }
func (s *fixtureSource) SourceKind() string             { return "linear" }
func (s *fixtureSource) Config() json.RawMessage        { return s.cfg }
func (s *fixtureSource) NangoConnectionID() string      { return "conn-linear-fixture" }
func (s *fixtureSource) NangoProviderConfigKey() string { return "linear" }

var _ interfaces.Source = (*fixtureSource)(nil)

// ---- fakeProxy: dispatches on operationName, serves stubbed responses ----

type stubResp struct {
	status int
	body   string
}

// fakeProxy is the shared test transport. Responses are stubbed per
// GraphQL operation name and served FIFO; once a queue drains, the last
// stubbed response for that op repeats (so pagination loops that call an
// op N times only need one trailing stub). Do errors if an op is hit
// before it was ever stubbed. Every raw request body is logged in
// `requests` for assertions.
type fakeProxy struct {
	mu       sync.Mutex
	queues   map[string][]stubResp
	last     map[string]stubResp
	requests [][]byte
}

func newFakeProxy() *fakeProxy {
	return &fakeProxy{
		queues: map[string][]stubResp{},
		last:   map[string]stubResp{},
	}
}

// stub enqueues a response for the given operation name.
func (f *fakeProxy) stub(opName string, status int, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queues[opName] = append(f.queues[opName], stubResp{status: status, body: body})
}

func (f *fakeProxy) Do(_ context.Context, body []byte) (int, http.Header, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.requests = append(f.requests, append([]byte(nil), body...))

	var envelope struct {
		OperationName string `json:"operationName"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return 0, nil, nil, fmt.Errorf("fakeProxy: undecodable request body: %w", err)
	}
	op := envelope.OperationName

	if q := f.queues[op]; len(q) > 0 {
		resp := q[0]
		f.queues[op] = q[1:]
		f.last[op] = resp
		return resp.status, http.Header{}, []byte(resp.body), nil
	}
	if resp, ok := f.last[op]; ok {
		return resp.status, http.Header{}, []byte(resp.body), nil
	}
	return 0, nil, nil, fmt.Errorf("fakeProxy: no stub for operation %q", op)
}

// requestBodies returns a copy of the logged raw request bodies as
// strings, for substring assertions.
func (f *fakeProxy) requestBodies() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.requests))
	for i, b := range f.requests {
		out[i] = string(b)
	}
	return out
}
