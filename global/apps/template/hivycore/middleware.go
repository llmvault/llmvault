// DO NOT EDIT — hivycore is the Hivy platform core. See doc.go.

package hivycore

import (
	"context"
	"net/http"
	"strings"
)

type contextKey int

const sessionContextKey contextKey = iota

// UserFrom returns the authenticated session attached to the request
// context. Under /api/* the middleware guarantees it is present; handlers
// may rely on ok being true there.
func UserFrom(ctx context.Context) (*Session, bool) {
	s, ok := ctx.Value(sessionContextKey).(*Session)
	return s, ok && s != nil
}

func withSession(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, sessionContextKey, s)
}

// sessionFromRequest decrypts the session cookie; any failure (absent,
// tampered, expired) is simply "no session".
func (a *App) sessionFromRequest(r *http.Request) *Session {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil
	}
	session, err := a.codec.decrypt(cookie.Value)
	if err != nil {
		return nil
	}
	return session
}

// requireAPISession guards /api/*: no valid session means 401 JSON carrying
// the launch URL so the SPA can send the top-level window back through the
// Hivy launch flow (the app itself never redirects).
func (a *App) requireAPISession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := a.sessionFromRequest(r)
		if session == nil {
			WriteJSON(w, http.StatusUnauthorized, map[string]string{
				"error":      "unauthorized",
				"launch_url": a.cfg.LaunchURL,
			})
			return
		}
		next.ServeHTTP(w, r.WithContext(withSession(r.Context(), session)))
	})
}

// pageGate fronts the static SPA: unauthenticated GET page navigations
// (Accept: text/html) get the 401 "not signed in" page — never a redirect;
// re-auth is the Hivy frontend's job (it mints a launch token and mounts
// the app iframe at /auth/callback). Everything else is served as-is (SPA
// assets are public code, and the API layer has its own guard).
func (a *App) pageGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := a.sessionFromRequest(r)
		if session == nil && r.Method == http.MethodGet && acceptsHTML(r) {
			a.writeNotAuthenticated(w)
			return
		}
		if session != nil {
			r = r.WithContext(withSession(r.Context(), session))
		}
		next.ServeHTTP(w, r)
	})
}

func acceptsHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}
