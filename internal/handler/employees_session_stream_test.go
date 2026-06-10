package handler

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
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
