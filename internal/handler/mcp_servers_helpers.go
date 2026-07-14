package handler

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *MCPServerHandler) view(r *http.Request, server model.MCPServer, actor *access.Actor, userID *uuid.UUID) (mcpServerResponse, error) {
	includeService := actor != nil && actor.IsOrgManager()
	user, service, err := h.service.AuthorizationSummaries(r.Context(), server.OrgID, server.ID, userID, includeService)
	if err != nil {
		return mcpServerResponse{}, err
	}
	return mcpServerView(server, user, service), nil
}

func (h *MCPServerHandler) requestContext(w http.ResponseWriter, r *http.Request, requireUser bool) (*model.Org, *access.Actor, *uuid.UUID, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return nil, nil, nil, false
	}
	userID, _ := currentRequestUserID(r.Context())
	if userID == nil {
		if requireUser {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing user context"})
			return nil, nil, nil, false
		}
		return org, nil, nil, true
	}
	actor, err := access.Resolve(r.Context(), h.db, org.ID, userID.String())
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid user context"})
		return nil, nil, nil, false
	}
	return org, actor, userID, true
}

func mcpPathID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid " + strings.ReplaceAll(name, "ID", " id")})
		return uuid.Nil, false
	}
	return id, true
}
