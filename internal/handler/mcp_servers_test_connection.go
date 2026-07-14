package handler

import (
	"net/http"

	"github.com/usehivy/hivy/internal/mcpservers"
)

type mcpConnectionTestResponse struct {
	Test mcpservers.ConnectionTestResult `json:"test"`
}

// Test handles POST /v1/mcp-servers/{id}/test.
// @Summary Test MCP server initialization and authorization
// @Tags mcp-servers
// @Produce json
// @Param id path string true "MCP server ID"
// @Success 200 {object} mcpConnectionTestResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 422 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/mcp-servers/{id}/test [post]
func (h *MCPServerHandler) Test(w http.ResponseWriter, r *http.Request) {
	org, _, userID, ok := h.requestContext(w, r, false)
	if !ok {
		return
	}
	serverID, ok := mcpPathID(w, r, "id")
	if !ok {
		return
	}
	server, err := h.service.GetServer(r.Context(), org.ID, serverID, userID)
	if err != nil {
		writeMCPError(w, r, err)
		return
	}
	result, err := h.service.TestConnection(r.Context(), *server, userID)
	if err != nil {
		writeMCPError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mcpConnectionTestResponse{Test: *result})
}
