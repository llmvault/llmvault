package email

import (
	"strings"
	"testing"
)

func TestRender_SubstitutesPlaceholders(t *testing.T) {
	got, err := Render(TmplAuthOtpLogin, TemplateVars{
		"code":      "123456",
		"email":     "user@example.com",
		"expiresIn": "10 minutes",
	}, "https://acme.test", "https://assets.acme.test")
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if !strings.Contains(got.HTML, "123456") {
		t.Errorf("HTML missing substituted code:\n%s", got.HTML)
	}
	if strings.Contains(got.HTML, "{{{code}}}") || strings.Contains(got.HTML, "{{{siteUrl}}}") || strings.Contains(got.HTML, "{{{assetBaseUrl}}}") {
		t.Errorf("HTML still contains unsubstituted placeholders:\n%s", got.HTML)
	}
	if !strings.Contains(got.HTML, "https://acme.test") {
		t.Errorf("HTML missing injected siteUrl")
	}
	if !strings.Contains(got.HTML, "https://assets.acme.test/hivy-logo.png") {
		t.Errorf("HTML missing injected logo asset URL")
	}
	if got.Subject != "Your Hivy login code: 123456" {
		t.Errorf("subject = %q", got.Subject)
	}
	if got.Text == "" {
		t.Errorf("expected a plaintext alternative")
	}
}

func TestRender_MissingVariableErrors(t *testing.T) {
	_, err := Render(TmplAuthOtpLogin, TemplateVars{"code": "123456"}, "https://acme.test", "https://assets.acme.test")
	if err == nil {
		t.Fatal("expected error for missing required variables")
	}
}

func TestRender_AllTemplatesEmbedded(t *testing.T) {
	for _, slug := range AllSlugs() {
		if _, err := templateFS.ReadFile("templates/dist/" + string(slug) + ".html"); err != nil {
			t.Errorf("template %s not embedded: %v", slug, err)
		}
	}
}
