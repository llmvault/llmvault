package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/counter"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/tasks"
)

func setupProxyAndAuxRoutes(
	r chi.Router,
	cfg *config.Config,
	deps *bootstrap.Deps,
	signingKey []byte,
	database *gorm.DB,
	proxyHandler http.Handler,
	auditWriter *middleware.AuditWriter,
	generationWriter *middleware.GenerationWriter,
	ctr *counter.Counter,
	enqueuer enqueue.TaskEnqueuer,
	runtimeCompileDeps agentruntime.CompileDeps,
) {
	var tokenAuthOpts []middleware.TokenAuthOption
	if enqueuer != nil {
		var inspector enqueue.TaskInspector
		if taskInspector, ok := enqueuer.(enqueue.TaskInspector); ok {
			inspector = taskInspector
		}
		tokenAuthOpts = append(tokenAuthOpts, middleware.WithExpiredProxyTokenHandler(
			tasks.NewExpiredProxyTokenRefreshScheduler(database, enqueuer, inspector, runtimeCompileDeps),
		))
	}

	r.Route("/v1/proxy", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, database, tokenAuthOpts...))
		r.Use(middleware.RequireCredits(deps.Credits))
		r.Use(middleware.RemainingCheck(ctr))
		r.Use(middleware.Audit(auditWriter, "proxy.request"))
		r.Use(middleware.Generation(generationWriter, database))
		r.Handle("/*", proxyHandler)
	})
}
