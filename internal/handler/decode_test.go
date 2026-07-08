package handler_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

// decodeInto unmarshals an httptest response body into dst, failing the test on
// error. Shared across handler_test files (previously lived in the now-removed
// channels_rag_sources_test.go).
func decodeInto(t *testing.T, rr *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rr.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode response: %v\n%s", err, rr.Body.String())
	}
}
