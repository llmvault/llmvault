// Package api holds this app's HTTP handlers. This is the agent-written
// layer: add handlers here and register them in Register. Every route is
// mounted under /api with authentication pre-applied, so
// hivycore.UserFrom(r.Context()) always returns the signed-in user.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"hivyapp/hivycore"
)

// Register mounts the app's API routes. Patterns are Go ServeMux patterns
// relative to /api ("GET /me" serves GET /api/me).
func Register(app *hivycore.App) {
	app.HandleAPI("GET /me", handleMe(app))
	app.HandleAPI("GET /pages", handlePages(app))
	app.HandleAPI("POST /pages/{pageID}/rows/query", handleQueryRows(app))
}

// handleMe returns the signed-in user from the session — the canonical
// "who am I" endpoint the SPA calls on load.
func handleMe(app *hivycore.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := hivycore.UserFrom(r.Context())
		if !ok {
			// Unreachable behind the auth middleware, but fail closed.
			app.WriteError(w, r, http.ErrNoCookie)
			return
		}
		hivycore.WriteJSON(w, http.StatusOK, user)
	}
}

// handlePages proxies the bound sheet's structure (pages, fields, row
// counts) to the SPA. This is the handler style to copy: take the request
// context, call the sheets client, relay errors through app.WriteError.
func handlePages(app *hivycore.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		structure, err := app.Sheets().Structure(r.Context())
		if err != nil {
			app.WriteError(w, r, err)
			return
		}
		hivycore.WriteJSON(w, http.StatusOK, structure)
	}
}

// handleQueryRows runs a filtered, sorted, paged rows query for one page. It
// backs the SPA's canonical useRows hook (web/src/hooks/queries.ts): an empty
// body means the default (unfiltered, default-sorted) query. Copy this shape
// when you expose more row reads.
func handleQueryRows(app *hivycore.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var query hivycore.Query
		if r.Body != nil {
			defer r.Body.Close()
			if err := json.NewDecoder(r.Body).Decode(&query); err != nil && !errors.Is(err, io.EOF) {
				hivycore.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid query body"})
				return
			}
		}
		result, err := app.Sheets().QueryRows(r.Context(), r.PathValue("pageID"), query)
		if err != nil {
			app.WriteError(w, r, err)
			return
		}
		hivycore.WriteJSON(w, http.StatusOK, result)
	}
}
