package handler

import (
	"errors"
	"net/http"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/mcpservers"
)

// writeMCPError is the single REST mapping for MCP control-plane service
// errors. Unknown failures are logged with their raw error and return only a
// static message so database, OAuth, and encryption details never leak.
func writeMCPError(w http.ResponseWriter, r *http.Request, err error) {
	var validation *mcpservers.ValidationError
	switch {
	case errors.As(err, &validation):
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: validation.Message})
	case errors.Is(err, mcpservers.ErrNotFound),
		errors.Is(err, mcpservers.ErrTeamNotFound),
		errors.Is(err, mcpservers.ErrAgentNotFound),
		errors.Is(err, mcpservers.ErrAuthorizationNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "MCP resource not found"})
	case errors.Is(err, mcpservers.ErrConflict):
		writeJSON(w, http.StatusConflict, errorResponse{Error: "MCP server already exists"})
	case errors.Is(err, mcpservers.ErrOAuthStateInvalid):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "OAuth state is invalid or expired"})
	default:
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "mcp control plane request failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "MCP request failed"})
	}
}
