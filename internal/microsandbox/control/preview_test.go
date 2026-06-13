package control

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/microsandbox/config"
)

func TestPreviewTokenIsBoundToSandbox(t *testing.T) {
	s := &Server{cfg: config.Config{PreviewJWTSecret: "test-secret"}}
	token, err := s.signPreviewToken("sbx_one", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !s.validPreviewToken(token, "sbx_one") {
		t.Fatal("token should be valid for original sandbox")
	}
	if s.validPreviewToken(token, "sbx_two") {
		t.Fatal("token should not be valid for another sandbox")
	}
}

func TestPreviewCookieUnlocksSandboxAcrossPorts(t *testing.T) {
	s := &Server{cfg: config.Config{PreviewJWTSecret: "test-secret", PreviewCookieTTL: time.Hour, PreviewCookieDomain: ".preview.test"}}
	rec := httptest.NewRecorder()
	s.setPreviewCookie(rec, "sbx_cookie")
	resp := rec.Result()
	defer resp.Body.Close()
	if len(resp.Cookies()) != 1 {
		t.Fatalf("cookies = %d, want 1", len(resp.Cookies()))
	}
	req := httptest.NewRequest(http.MethodGet, "https://3000-sbx_cookie.preview.test/", nil)
	req.AddCookie(resp.Cookies()[0])
	if !s.previewCookieValid(req, "sbx_cookie") {
		t.Fatal("cookie should unlock same sandbox")
	}
	if s.previewCookieValid(req, "sbx_other") {
		t.Fatal("cookie should not unlock another sandbox")
	}
}

func TestParsePreviewHost(t *testing.T) {
	port, sandboxID, ok := parsePreviewHost("3000-sbx_abc.preview.usehivy.test")
	if !ok || port != 3000 || sandboxID != "sbx_abc" {
		t.Fatalf("parsePreviewHost got port=%d sandbox=%q ok=%v", port, sandboxID, ok)
	}
}
