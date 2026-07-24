package metrics

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

// HTTPMiddleware records RED metrics using Chi's normalized route pattern.
func HTTPMiddleware(service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL != nil && (r.URL.Path == "/metrics" || r.URL.Path == "/internal/metrics") {
				next.ServeHTTP(w, r)
				return
			}
			started := time.Now()
			finish := TrackHTTPRequest(service)
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			finish(routePattern(r), r.Method, ww.Status(), time.Since(started))
		})
	}
}

func routePattern(r *http.Request) string {
	if r == nil {
		return "unmatched"
	}
	if routeCtx := chi.RouteContext(r.Context()); routeCtx != nil {
		if pattern := routeCtx.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return "unmatched"
}
