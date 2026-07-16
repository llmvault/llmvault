package model

import (
	"strings"

	"github.com/google/uuid"
)

// ConnectionMCPToolDeny stores agent-specific tool opt-outs keyed by concrete
// connection UUID. Missing connections and tool names are allowed by default.
type ConnectionMCPToolDeny map[string][]string

// ConnectionMCPToolDenyAll disables the inherited connection itself for one
// agent. Other entries continue to deny individual generated MCP tools.
const ConnectionMCPToolDenyAll = "*"

func (deny ConnectionMCPToolDeny) DisablesConnection(connectionID uuid.UUID) bool {
	if connectionID == uuid.Nil {
		return false
	}
	for _, tool := range deny[connectionID.String()] {
		if strings.TrimSpace(tool) == ConnectionMCPToolDenyAll {
			return true
		}
	}
	return false
}

func (deny ConnectionMCPToolDeny) DisabledConnectionIDs() []uuid.UUID {
	ids := make([]uuid.UUID, 0)
	for rawID := range deny {
		id, err := uuid.Parse(rawID)
		if err == nil && deny.DisablesConnection(id) {
			ids = append(ids, id)
		}
	}
	return ids
}
