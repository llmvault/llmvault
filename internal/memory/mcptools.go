package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

const (
	memoryToolSearchLimit   = 10
	memoryToolQueryMaxWords = 6
	memoryToolQueryMaxChars = 40
	memoryToolMaxTags       = 5
)

// NewToolsFunc registers the privileged org-wide memory view for eligible
// agent proxy MCP servers.
//
// Agents are read-only on memory: every write flows through background
// reflection/consolidation, and corrections or deletions happen in the
// memories UI. The privileged manage_memories tool (also read-only: search +
// overview) additionally requires the calling agent to be the org's default
// agent; the agent row is loaded once here to evaluate that gate.

func NewToolsFunc(service *Service) func(server *mcp.Server, token *model.Token) {
	return func(server *mcp.Server, token *model.Token) {
		if server == nil || service == nil || service.cfg.DB == nil || !memoryToolAgentProxy(token) {
			return
		}
		agentID, err := memoryToolAgentID(token)
		if err != nil {
			return
		}
		manage := false
		if agent, err := service.loadOrgAgent(context.Background(), token.OrgID, agentID); err == nil {
			manage = agent.IsDefault
		}
		if manage {
			registerManageMemoriesTool(server, service, token)
		}
	}
}

func decodeMemoryToolArgs(req *mcp.CallToolRequest, dst any) error {
	if req == nil || req.Params.Arguments == nil {
		return fmt.Errorf("arguments are required")
	}
	if err := json.Unmarshal(req.Params.Arguments, dst); err != nil {
		return fmt.Errorf("invalid arguments")
	}
	return nil
}
