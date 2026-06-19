package control

import (
	"net/http"

	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/httpx"
)

type execRequest struct {
	Command        string `json:"command"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

func (s *Server) execSandbox(w http.ResponseWriter, r *http.Request) {
	sb, runner, ok := s.loadSandboxRunner(w, r)
	if !ok {
		return
	}
	var req execRequest
	if err := httpx.Decode(r, &req); err != nil || req.Command == "" {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "command is required"})
		return
	}
	var out map[string]any
	if err := s.client.Post(r.Context(), runner.APIURL, "/v1/sandboxes/"+sb.ID+"/exec", req, &out); err != nil {
		httpx.JSON(w, http.StatusBadGateway, api.ErrorResponse{Error: err.Error()})
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (s *Server) logsSandbox(w http.ResponseWriter, r *http.Request) {
	sb, runner, ok := s.loadSandboxRunner(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err := s.client.Get(r.Context(), runner.APIURL, "/v1/sandboxes/"+sb.ID+"/logs", w); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
}
