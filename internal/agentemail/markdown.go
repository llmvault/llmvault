package agentemail

import (
	"bytes"
	"errors"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

const emailDocumentPrefix = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>
body{margin:0;padding:0;background:#f4f4f5;color:#18181b;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;line-height:1.6}
.email-shell{max-width:720px;margin:0 auto;padding:32px 20px}
.email-content{background:#fff;border:1px solid #e4e4e7;border-radius:12px;padding:32px}
h1,h2,h3,h4,h5,h6{line-height:1.25;margin:1.4em 0 .55em}h1{font-size:26px}h2{font-size:22px}h3{font-size:18px}
p,ul,ol,blockquote,pre,table{margin:0 0 16px}a{color:#2563eb}blockquote{border-left:4px solid #d4d4d8;margin-left:0;padding-left:16px;color:#52525b}
code{background:#f4f4f5;border-radius:4px;padding:2px 5px;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:13px}pre{background:#18181b;color:#fafafa;border-radius:8px;overflow:auto;padding:16px}pre code{background:transparent;padding:0}
table{border-collapse:collapse;width:100%}th,td{border:1px solid #d4d4d8;padding:8px 10px;text-align:left;vertical-align:top}th{background:#f4f4f5}hr{border:0;border-top:1px solid #e4e4e7;margin:24px 0}
@media(max-width:600px){.email-shell{padding:12px}.email-content{border-radius:8px;padding:20px}}
</style>
</head>
<body><div class="email-shell"><main class="email-content">`

const emailDocumentSuffix = `</main></div></body></html>`

var markdownRenderer = goldmark.New(goldmark.WithExtensions(extension.GFM))

type normalizedEmailBodies struct {
	text string
	html string
}

func normalizeEmailBodies(markdown, text, html string) (normalizedEmailBodies, error) {
	if strings.TrimSpace(markdown) != "" {
		if strings.TrimSpace(text) != "" || strings.TrimSpace(html) != "" {
			return normalizedEmailBodies{}, errors.New("provide markdown or text/html, not both")
		}
		if len(markdown) > maxEmailBody {
			return normalizedEmailBodies{}, errors.New("markdown must be at most 1 MiB")
		}
		rendered, err := renderMarkdownEmail(markdown)
		if err != nil {
			return normalizedEmailBodies{}, errors.New("failed to render markdown email")
		}
		return normalizedEmailBodies{text: markdown, html: rendered}, nil
	}
	if strings.TrimSpace(text) == "" && strings.TrimSpace(html) == "" {
		return normalizedEmailBodies{}, errors.New("provide markdown, text, or html")
	}
	if len(text)+len(html) > maxEmailBody {
		return normalizedEmailBodies{}, errors.New("text and html must have a combined maximum of 1 MiB")
	}
	return normalizedEmailBodies{text: text, html: html}, nil
}

func renderMarkdownEmail(markdown string) (string, error) {
	var rendered bytes.Buffer
	if err := markdownRenderer.Convert([]byte(markdown), &rendered); err != nil {
		return "", err
	}
	// Raw Markdown HTML is disabled by Goldmark. The explicit allowlist is a
	// second boundary for links and all generated markup before email delivery.
	sanitized := markdownEmailPolicy().SanitizeBytes(rendered.Bytes())
	return emailDocumentPrefix + string(sanitized) + emailDocumentSuffix, nil
}

func markdownEmailPolicy() *bluemonday.Policy {
	policy := bluemonday.NewPolicy()
	policy.AllowElements(
		"p", "br", "hr", "h1", "h2", "h3", "h4", "h5", "h6",
		"ul", "ol", "li", "strong", "b", "em", "i", "del", "s",
		"blockquote", "pre", "code", "table", "thead", "tbody", "tfoot",
		"tr", "th", "td", "a",
	)
	policy.AllowAttrs("href", "title").OnElements("a")
	policy.AllowURLSchemes("http", "https", "mailto")
	policy.RequireNoFollowOnLinks(true)
	policy.RequireNoReferrerOnLinks(true)
	return policy
}
