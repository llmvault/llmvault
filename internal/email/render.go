package email

import (
	"embed"
	"fmt"
	"strings"
)

// templateFS holds the HTML/plaintext bodies built from the React Email sources
// under templates/ (run `make emails` / the templates build). Each file is named
// <slug>.html / <slug>.txt and contains {{{variable}}} placeholders that Render
// substitutes at send time.
//
//go:embed templates/dist/*.html templates/dist/*.txt
var templateFS embed.FS

// RenderedEmail is a fully-compiled email ready for a transport to deliver.
type RenderedEmail struct {
	Subject string
	HTML    string
	Text    string
}

// Render compiles a template into its final subject, HTML and plaintext bodies
// by substituting {{{variable}}} placeholders. siteURL and assetBaseURL are
// injected as the {{{siteUrl}}} / {{{assetBaseUrl}}} placeholders used by the
// shared email shell (footer links + logo image).
func Render(slug TemplateSlug, vars TemplateVars, siteURL, assetBaseURL string) (RenderedEmail, error) {
	if missing := Validate(slug, vars); len(missing) > 0 {
		return RenderedEmail{}, fmt.Errorf("email: template %s missing variables: %v", slug, missing)
	}

	html, err := templateFS.ReadFile("templates/dist/" + string(slug) + ".html")
	if err != nil {
		return RenderedEmail{}, fmt.Errorf("email: load html template %s: %w", slug, err)
	}
	// Plaintext body is optional; absence just yields an HTML-only email.
	text, _ := templateFS.ReadFile("templates/dist/" + string(slug) + ".txt")

	all := make(TemplateVars, len(vars)+2)
	for k, v := range vars {
		all[k] = v
	}
	all["siteUrl"] = siteURL
	all["assetBaseUrl"] = assetBaseURL

	return RenderedEmail{
		Subject: Subject(slug, vars),
		HTML:    substitutePlaceholders(string(html), all),
		Text:    substitutePlaceholders(string(text), all),
	}, nil
}

// substitutePlaceholders replaces {{{key}}} (and {{key}}) with the given values.
func substitutePlaceholders(body string, vars TemplateVars) string {
	if body == "" {
		return ""
	}
	for k, v := range vars {
		body = strings.ReplaceAll(body, "{{{"+k+"}}}", v)
		body = strings.ReplaceAll(body, "{{"+k+"}}", v)
	}
	return body
}
