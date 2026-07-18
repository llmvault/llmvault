package memory

import (
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

// NewToolsFunc registers the agent-scoped, read-only memory view for agent
// proxy MCP servers. Memory writes flow exclusively through reflection and
// consolidation.

func NewToolsFunc(service *Service) func(server *mcp.Server, token *model.Token) {
	return func(server *mcp.Server, token *model.Token) {
		if server == nil || service == nil || service.cfg.DB == nil || !memoryToolAgentProxy(token) {
			return
		}
		agentID, err := memoryToolAgentID(token)
		if err != nil {
			return
		}
		registerManageMemoriesTool(server, service, token, agentID)
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
