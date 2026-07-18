package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// authorizeSandboxOp loads the sandbox for a mutating op (stop/exec/delete),
// scoped to the org and — for non-manager callers — to sandboxes whose agent is
// visible to them (the same predicate the read Get path uses). A hidden or
// agent-less sandbox is indistinguishable from a nonexistent one (404), so
// arbitrary code exec and teardown are unreachable for a sandbox the caller
// cannot see. Managers and API-key callers reach any sandbox in the org.
func (h *SandboxHandler) authorizeSandboxOp(w http.ResponseWriter, r *http.Request) (model.Sandbox, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return model.Sandbox{}, false
	}
	orgWide, userID, err := actorSeesOrgWide(r.Context(), h.db, org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve access"})
		return model.Sandbox{}, false
	}
	id := chi.URLParam(r, "id")
	q := h.db.Where("id = ? AND org_id = ?", id, org.ID)
	if !orgWide {
		q = q.Where("agent_id IN (SELECT id FROM agents WHERE team_id IN (?))", visibleTeamSubquery(h.db, userID))
	}
	var sb model.Sandbox
	if err := q.First(&sb).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return model.Sandbox{}, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox"})
		return model.Sandbox{}, false
	}
	return sb, true
}

// Stop handles POST /v1/sandboxes/{id}/stop.
// @Summary Stop a sandbox
// @Description Stops a running sandbox via the sandbox provider.
// @Tags sandboxes
// @Produce json
// @Param id path string true "Sandbox ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sandboxes/{id}/stop [post]
func (h *SandboxHandler) Stop(w http.ResponseWriter, r *http.Request) {
	if h.orchestrator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sandbox orchestrator not configured"})
		return
	}
	sb, ok := h.authorizeSandboxOp(w, r)
	if !ok {
		return
	}
	if err := h.orchestrator.StopSandbox(r.Context(), &sb); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to stop sandbox"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// Delete handles DELETE /v1/sandboxes/{id}.
// @Summary Delete a sandbox
// @Description Deletes a sandbox from the provider and removes the DB record.
// @Tags sandboxes
// @Produce json
// @Param id path string true "Sandbox ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sandboxes/{id} [delete]
func (h *SandboxHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if h.orchestrator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sandbox orchestrator not configured"})
		return
	}
	sb, ok := h.authorizeSandboxOp(w, r)
	if !ok {
		return
	}
	if err := h.orchestrator.DeleteSandbox(r.Context(), &sb); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete sandbox"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type execRequest struct {
	Commands []string `json:"commands"`
}

type commandResult struct {
	Command  string `json:"command"`
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

type execResponse struct {
	Results []commandResult `json:"results"`
	Success bool            `json:"success"`
}

// Exec handles POST /v1/sandboxes/{id}/exec.
// @Summary Execute commands in a sandbox
// @Description Runs an array of shell commands sequentially inside the sandbox. Stops on first failure.
// @Tags sandboxes
// @Accept json
// @Produce json
// @Param id path string true "Sandbox ID"
// @Param body body execRequest true "Commands to execute"
// @Success 200 {object} execResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sandboxes/{id}/exec [post]
func (h *SandboxHandler) Exec(w http.ResponseWriter, r *http.Request) {
	sb, ok := h.authorizeSandboxOp(w, r)
	if !ok {
		return
	}
	var req execRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if len(req.Commands) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "commands array is required and must not be empty"})
		return
	}

	if h.orchestrator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sandbox orchestrator not configured"})
		return
	}

	results := make([]commandResult, 0, len(req.Commands))
	allSuccess := true

	for _, cmd := range req.Commands {
		output, err := h.orchestrator.ExecuteCommand(r.Context(), &sb, cmd)
		result := commandResult{
			Command: cmd,
			Output:  output,
		}
		if err != nil {
			result.Error = err.Error()
			result.ExitCode = 1
			allSuccess = false
			logging.FromContext(r.Context()).DebugContext(r.Context(), "sandbox exec: command failed", "sandbox_id", sb.ID, "command", cmd, "error", err)
			results = append(results, result)
			break
		}
		results = append(results, result)
	}

	h.db.Model(&sb).Update("last_active_at", time.Now())

	writeJSON(w, http.StatusOK, execResponse{
		Results: results,
		Success: allSuccess,
	})
}
