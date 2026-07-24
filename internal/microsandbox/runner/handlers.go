package runner

import (
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/usehivy/hivy/internal/microsandbox/httpx"
	"github.com/usehivy/hivy/internal/observability/correlation"
)

func (s *Server) createSandbox(w http.ResponseWriter, r *http.Request) {
	var req CreateSandboxRequest
	if err := httpx.Decode(r, &req); err != nil || req.ID == "" || req.ImageRef == "" {
		httpx.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	values := correlation.Merge(correlation.FromHeaders(r.Header), correlation.FromLabels(req.Labels))
	values.SandboxID = req.ID
	ctx := correlation.WithValues(r.Context(), values)
	if err := s.addSandboxLogIngestEnv(&req); err != nil {
		httpx.JSON(w, http.StatusInternalServerError, map[string]string{"error": "sandbox log ingestion is not configured"})
		return
	}
	done := s.trackStartingOperation()
	defer done()
	resp, err := s.backend.CreateSandbox(ctx, req)
	if err != nil {
		httpx.JSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	httpx.JSON(w, http.StatusCreated, resp)
}

func (s *Server) startSandbox(w http.ResponseWriter, r *http.Request) {
	done := s.trackStartingOperation()
	defer done()
	if err := s.backend.StartSandbox(r.Context(), chi.URLParam(r, "sandboxID")); err != nil {
		httpx.JSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "running"})
}

func (s *Server) stopSandbox(w http.ResponseWriter, r *http.Request) {
	if err := s.backend.StopSandbox(r.Context(), chi.URLParam(r, "sandboxID")); err != nil {
		httpx.JSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) ensureSandboxReady(w http.ResponseWriter, r *http.Request) {
	var req EnsureReadyRequest
	if err := httpx.Decode(r, &req); err != nil || req.GuestPort <= 0 {
		httpx.JSON(w, http.StatusBadRequest, map[string]string{"error": "valid guest_port is required"})
		return
	}
	done := s.trackStartingOperation()
	defer done()
	resp, err := s.backend.EnsureReady(r.Context(), chi.URLParam(r, "sandboxID"), req)
	if err != nil {
		httpx.JSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

func (s *Server) sandboxConnections(w http.ResponseWriter, r *http.Request) {
	resp, err := s.backend.Connections(r.Context(), chi.URLParam(r, "sandboxID"))
	if err != nil {
		httpx.JSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

func (s *Server) deleteSandbox(w http.ResponseWriter, r *http.Request) {
	if err := s.backend.DeleteSandbox(r.Context(), chi.URLParam(r, "sandboxID")); err != nil {
		httpx.JSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) execSandbox(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := httpx.Decode(r, &req); err != nil || req.Command == "" {
		httpx.JSON(w, http.StatusBadRequest, map[string]string{"error": "command is required"})
		return
	}
	resp, err := s.backend.Exec(r.Context(), chi.URLParam(r, "sandboxID"), req.Command, req.TimeoutSeconds)
	if err != nil {
		httpx.JSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

func (s *Server) logsSandbox(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err := s.backend.Logs(r.Context(), chi.URLParam(r, "sandboxID"), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) createTemplate(w http.ResponseWriter, r *http.Request) {
	var req CreateTemplateRequest
	if err := httpx.Decode(r, &req); err != nil || req.ID == "" || req.OrgID == "" || req.BaseImageRef == "" {
		httpx.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	flush := func() {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	emit := func(event TemplateBuildEvent) {
		_ = enc.Encode(event)
		flush()
	}
	emit(TemplateBuildEvent{Type: "status", Status: "building", ID: req.ID})
	resp, err := s.backend.CreateTemplate(r.Context(), req, emit)
	if err != nil {
		emit(TemplateBuildEvent{Type: "error", Status: "error", ID: req.ID, Message: err.Error()})
		return
	}
	emit(TemplateBuildEvent{
		Type: "result", Status: "ready", ID: resp.ID, ImageRef: resp.ImageRef, ImageDigest: resp.ImageDigest,
		ValidationSandboxID: resp.ValidationSandboxID,
	})
}

func (s *Server) proxySandbox() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sandboxID := chi.URLParam(r, "sandboxID")
		port, err := strconv.Atoi(chi.URLParam(r, "port"))
		if err != nil {
			http.Error(w, "invalid port", http.StatusBadRequest)
			return
		}
		raw, err := s.backend.ProxyURL(r.Context(), sandboxID, port)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		target, err := url.Parse(raw)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		p := httputil.NewSingleHostReverseProxy(target)
		p.Director = func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.URL.Path = "/" + chi.URLParam(r, "*")
			req.URL.RawQuery = r.URL.RawQuery
			req.Host = target.Host
		}
		p.ServeHTTP(w, r)
	})
}
