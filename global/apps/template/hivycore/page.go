// DO NOT EDIT — hivycore is the Hivy platform core. See doc.go.

package hivycore

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
)

// frameAncestorsFrom derives the Content-Security-Policy value stamped on
// every HTML response. Apps render only inside the Hivy frontend's iframe,
// so the allowed embedders are the app itself and the Hivy frontend origin
// (scheme://host of the launch URL). An unset or unparseable launch URL
// falls back to 'self' alone. X-Frame-Options is deliberately never set: it
// cannot allow one specific origin and would fight this policy.
func frameAncestorsFrom(launchURL string) string {
	const self = "frame-ancestors 'self'"
	u, err := url.Parse(launchURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return self
	}
	return self + " " + u.Scheme + "://" + u.Host
}

// setHTMLHeaders stamps the headers every HTML response carries — the
// embedding policy that lets the Hivy frontend (and nobody else) iframe
// this app.
func (a *App) setHTMLHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", a.frameAncestors)
}

// writeNotAuthenticated serves the minimal "not signed in" page: 401 with a
// self-contained HTML body and a plain link to the Hivy launch URL. Apps are
// opened only from the Hivy frontend, which fetches a launch token and
// mounts the app in an iframe at /auth/callback — so the app itself never
// redirects anywhere; this page is the terminal answer to any
// unauthenticated navigation or failed callback.
func (a *App) writeNotAuthenticated(w http.ResponseWriter) {
	a.setHTMLHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprintf(w, notAuthenticatedPage, html.EscapeString(a.cfg.LaunchURL))
}

const notAuthenticatedPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Not signed in</title>
</head>
<body style="margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;font-family:system-ui,-apple-system,sans-serif;background:#fafafa;color:#18181b">
<main style="text-align:center;padding:2rem;max-width:24rem">
<h1 style="font-size:1.125rem;font-weight:600;margin:0 0 0.5rem">You&#39;re not signed in</h1>
<p style="font-size:0.875rem;color:#52525b;margin:0 0 1.25rem">Open this app from Hivy.</p>
<a href="%s" style="font-size:0.875rem;color:#2563eb;text-decoration:underline">Open in Hivy</a>
</main>
</body>
</html>
`
