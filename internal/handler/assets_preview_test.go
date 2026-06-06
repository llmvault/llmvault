package handler_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPreviewAsset_RedirectsToSignedGetURL(t *testing.T) {
	h := newStreamHarness(t)
	publicURL := h.seedEmployeeAsset(t, "previews", "demo.txt", "preview body")

	u, err := url.Parse(publicURL)
	if err != nil {
		t.Fatalf("parse preview URL: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, u.RequestURI(), nil)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("unexpected cache-control: %q", rr.Header().Get("Cache-Control"))
	}
	location := rr.Header().Get("Location")
	if location == "" {
		t.Fatalf("missing redirect location")
	}
	if !strings.Contains(location, "X-Amz-Signature=") {
		t.Fatalf("expected signed S3 URL, got %q", location)
	}
}

func TestPreviewAsset_RejectsInvalidPaths(t *testing.T) {
	h := newStreamHarness(t)
	for _, raw := range []string{
		"",
		"/pub/e/example/file.txt",
		"pub/e/example/../file.txt",
		"pub/e/example//file.txt",
		"private/e/example/file.txt",
		"pub/e/example\\file.txt",
	} {
		req := httptest.NewRequest(http.MethodGet, "/v1/assets/preview?path="+url.QueryEscape(raw), nil)
		rr := httptest.NewRecorder()
		h.router.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("path %q expected 400, got %d: %s", raw, rr.Code, rr.Body.String())
		}
	}
}
