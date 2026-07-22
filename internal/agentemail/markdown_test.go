package agentemail

import (
	"strings"
	"testing"
)

func TestNormalizeEmailBodiesRendersMarkdownForTextAndHTML(t *testing.T) {
	markdown := "# Cluster health\n\n| Service | State |\n| --- | --- |\n| API | Healthy |\n"

	bodies, err := normalizeEmailBodies(markdown, "", "")
	if err != nil {
		t.Fatalf("normalize Markdown email: %v", err)
	}
	if bodies.text != markdown {
		t.Fatalf("plain-text fallback = %q, want original Markdown", bodies.text)
	}
	for _, expected := range []string{"<!doctype html>", "<h1>Cluster health</h1>", "<table>", "<td>Healthy</td>"} {
		if !strings.Contains(bodies.html, expected) {
			t.Fatalf("rendered HTML does not contain %q: %s", expected, bodies.html)
		}
	}
}

func TestRenderMarkdownEmailSanitizesRawHTMLAndUnsafeLinks(t *testing.T) {
	rendered, err := renderMarkdownEmail("# Report\n\n<script>alert('x')</script>\n\n[unsafe](javascript:alert('x'))\n\n[safe](https://example.test/report)")
	if err != nil {
		t.Fatalf("render Markdown email: %v", err)
	}
	for _, forbidden := range []string{"<script", "alert('x')", "javascript:"} {
		if strings.Contains(strings.ToLower(rendered), forbidden) {
			t.Fatalf("rendered HTML contains unsafe value %q: %s", forbidden, rendered)
		}
	}
	if !strings.Contains(rendered, `href="https://example.test/report"`) {
		t.Fatalf("rendered HTML removed safe link: %s", rendered)
	}
}

func TestNormalizeEmailBodiesRejectsAmbiguousOrOversizeBody(t *testing.T) {
	if _, err := normalizeEmailBodies("# Report", "duplicate", ""); err == nil {
		t.Fatal("Markdown and legacy text were accepted together")
	}
	if _, err := normalizeEmailBodies(strings.Repeat("x", maxEmailBody+1), "", ""); err == nil {
		t.Fatal("oversize Markdown was accepted")
	}
	if _, err := normalizeEmailBodies("", "", ""); err == nil {
		t.Fatal("empty email body was accepted")
	}
}
