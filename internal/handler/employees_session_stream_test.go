package handler

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordingFlushWriter struct {
	header  http.Header
	body    bytes.Buffer
	flushes []int
}

func (w *recordingFlushWriter) Header() http.Header {
	return w.header
}

func (w *recordingFlushWriter) Write(p []byte) (int, error) {
	return w.body.Write(p)
}

func (w *recordingFlushWriter) WriteHeader(statusCode int) {}

func (w *recordingFlushWriter) Flush() {
	w.flushes = append(w.flushes, w.body.Len())
}

func TestCopySSEStreamFlushesAtEventBoundaries(t *testing.T) {
	body := strings.Join([]string{
		"event: token\n",
		"data: {\"text\":\"a\"}\n",
		"\n",
		"event: tool_call\n",
		"data: {\"id\":\"call-1\"}\n",
		"\n",
	}, "")
	writer := &recordingFlushWriter{header: http.Header{}}

	if err := copySSEStream(writer, strings.NewReader(body)); err != nil {
		t.Fatalf("copy stream: %v", err)
	}

	if got := writer.body.String(); got != body {
		t.Fatalf("body = %q, want %q", got, body)
	}
	if len(writer.flushes) < 2 {
		t.Fatalf("flushes = %d, want at least 2", len(writer.flushes))
	}
	if writer.flushes[0] >= writer.flushes[1] {
		t.Fatalf("flush positions were not event ordered: %#v", writer.flushes)
	}
}

// slowReader emits SSE chunks one at a time with a delay, simulating a slow runtime.
type slowReader struct {
	mu        sync.Mutex
	chunks    [][]byte
	delay     time.Duration
	idx       int
	readTimes []time.Time
	// failAfter, when >= 0, makes Read return errReadFailure after that many
	// chunks (simulating a transport drop mid-body).
	failAfter int
}

var errReadFailure = errors.New("simulated transport drop")

func (r *slowReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.idx > 0 || len(r.readTimes) > 0 {
		time.Sleep(r.delay)
	}
	if r.failAfter >= 0 && r.idx >= r.failAfter {
		return 0, errReadFailure
	}
	if r.idx >= len(r.chunks) {
		return 0, io.EOF
	}
	chunk := r.chunks[r.idx]
	r.idx++
	r.readTimes = append(r.readTimes, time.Now())
	n := copy(p, chunk)
	return n, nil
}

// timestampWriter records Flush times so a test can assert progressive forwarding.
type timestampWriter struct {
	header     http.Header
	body       bytes.Buffer
	flushTimes []time.Time
}

func (w *timestampWriter) Header() http.Header { return w.header }
func (w *timestampWriter) Write(p []byte) (int, error) {
	return w.body.Write(p)
}
func (w *timestampWriter) WriteHeader(int) {}
func (w *timestampWriter) Flush() {
	w.flushTimes = append(w.flushTimes, time.Now())
}

// The proxy must forward a slowly-streamed body chunk-by-chunk (flushing at each
// event boundary as it arrives) without buffering the whole turn or killing it.
func TestCopySSEStreamForwardsSlowChunksProgressively(t *testing.T) {
	const delay = 20 * time.Millisecond
	reader := &slowReader{
		delay:     delay,
		failAfter: -1,
		chunks: [][]byte{
			[]byte("event: token\ndata: {\"text\":\"a\"}\n\n"),
			[]byte("event: token\ndata: {\"text\":\"b\"}\n\n"),
			[]byte("event: token\ndata: {\"text\":\"c\"}\n\n"),
			[]byte("event: done\ndata: {}\n\n"),
		},
	}
	writer := &timestampWriter{header: http.Header{}}

	start := time.Now()
	if err := copySSEStream(writer, reader); err != nil {
		t.Fatalf("copy stream: %v", err)
	}

	body := writer.body.String()
	if strings.Count(body, "event: token") != 3 {
		t.Fatalf("expected 3 token events, body = %q", body)
	}
	if !strings.HasSuffix(strings.TrimRight(body, "\n"), "data: {}") {
		t.Fatalf("expected terminal done event last, body = %q", body)
	}
	if len(writer.flushTimes) < 4 {
		t.Fatalf("expected >= 4 flushes (one per event), got %d", len(writer.flushTimes))
	}
	// Elapsed time must reflect the per-chunk delays, proving progressive consumption.
	if elapsed := time.Since(start); elapsed < 3*delay {
		t.Fatalf("stream completed too fast (%v); expected progressive consumption", elapsed)
	}
}

// A transport drop mid-stream must surface as an error from the proxy copy
// (after partial bytes are flushed), not silently complete or hang, so the
// caller's request scope ends and the client can reconnect to the replay buffer.
func TestCopySSEStreamPropagatesMidBodyDropError(t *testing.T) {
	reader := &slowReader{
		delay:     5 * time.Millisecond,
		failAfter: 2, // emit two chunks, then drop.
		chunks: [][]byte{
			[]byte("event: token\ndata: {\"text\":\"a\"}\n\n"),
			[]byte("event: token\ndata: {\"text\":\"b\"}\n\n"),
			[]byte("event: done\ndata: {}\n\n"),
		},
	}
	writer := &timestampWriter{header: http.Header{}}

	err := copySSEStream(writer, reader)
	if !errors.Is(err, errReadFailure) {
		t.Fatalf("expected the mid-body drop error to propagate, got %v", err)
	}
	body := writer.body.String()
	if strings.Count(body, "event: token") != 2 {
		t.Fatalf("expected the two pre-drop tokens to be flushed, body = %q", body)
	}
	if strings.Contains(body, "event: done") {
		t.Fatalf("a dropped stream must not produce a terminal event, body = %q", body)
	}
}
