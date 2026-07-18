package handler

import (
	"net/http"
	"net/http/httputil"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/proxy"
)

// NewProxyHandler creates the streaming reverse proxy handler.
// It uses FlushInterval: -1 to immediately flush SSE chunks.
// The transport should be wrapped with proxy.CaptureTransport for observability.
func NewProxyHandler(db *gorm.DB, redisClient *redis.Client, cacheManager *cache.Manager, attrCache *middleware.AttributionCache, transport http.RoundTripper) http.Handler {
	router := proxy.NewModelRouter(db, redisClient, nil)
	director := proxy.NewDirector(cacheManager, attrCache, router)
	transport = &proxy.FailoverTransport{Inner: transport, Director: director, Router: router}

	rp := &httputil.ReverseProxy{
		Director:      director.Direct,
		Transport:     transport,
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {

			if proxyErr := r.Header.Get("X-Proxy-Error"); proxyErr != "" {
				http.Error(w, `{"error":"`+proxyErr+`"}`, http.StatusBadGateway)
				return
			}
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "proxy upstream error",
				"error", err,
				"method", r.Method,
				"path", r.URL.Path,
				"host", r.URL.Host,
			)
			http.Error(w, `{"error":"upstream unreachable"}`, http.StatusBadGateway)
		},
	}

	return rp
}
