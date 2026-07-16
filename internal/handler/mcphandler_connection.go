package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func connectionServerTarget(r *http.Request) (string, uuid.UUID, bool, error) {
	wildcard := strings.Trim(chi.URLParam(r, "*"), "/")
	if wildcard == "" {
		return "", uuid.Nil, false, nil
	}
	parts := strings.Split(wildcard, "/")
	if len(parts) != 2 || (parts[0] != "connection" && parts[0] != "database") {
		return "", uuid.Nil, false, fmt.Errorf("unknown MCP server path")
	}
	connectionID, err := uuid.Parse(parts[1])
	if err != nil || connectionID == uuid.Nil {
		return "", uuid.Nil, false, fmt.Errorf("invalid MCP connection id")
	}
	return parts[0], connectionID, true, nil
}
