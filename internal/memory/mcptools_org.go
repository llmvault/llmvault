package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/model"
)

const manageMemoriesToolName = "manage_memories"

const manageMemoriesTopTagsLimit = 25

// requireOrgManagerActor gates the org-wide memory manager on the acting human
// being an org owner/admin. A nil result means allowed. When there is no human
// actor (automated run, or a deploy before the runtime injects identity) the
// check is skipped so existing flows keep working. rawActorUserID is the
// runtime-injected `_hivy_actor_user_id`.
func (s *Service) requireOrgManagerActor(ctx context.Context, orgID uuid.UUID, rawActorUserID string) *mcp.CallToolResult {
	actor, err := access.Resolve(ctx, s.cfg.DB, orgID, rawActorUserID)
	if err != nil {
		return memoryToolError(err.Error())
	}
	if actor == nil {
		return nil
	}
	if !actor.IsOrgManager() {
		return memoryToolError("Not allowed: managing the organization's memories across all channels requires an admin or owner. " +
			"The person you're acting for has the role \"" + actor.OrgRole + "\", which can only manage this channel's own memories. " +
			"Ask an organization admin or owner if you need the org-wide view.")
	}
	return nil
}

const manageMemoriesDescription = `Org-wide memory manager across ALL channels and organization-wide memories. Available only to the org's default agent.

action search runs a semantic search over consolidated observations across every channel plus organization-wide memories, with the owning channel identified per result. Pass channel_id to narrow to one channel, or channel_id "org" for organization-wide memories only. query is a 2-6 word phrase; tags are optional exact lowercase slug filters. Set include_facts true to debug the raw extracted facts layer instead.

action overview returns aggregate counts: total memories, organization-wide count, per-channel counts, and top tags.

action forget archives any memory in the organization by memory_id. Observation IDs (what search returns) are archived and suppressed from being re-learned; raw fact IDs are archived.

action retain stores a memory into a specific channel (channel_id) or organization-wide (omit channel_id or pass "org").`

type manageMemoriesArgs struct {
	Action          string     `json:"action"`
	ChannelID       string     `json:"channel_id"`
	Query           string     `json:"query"`
	Tags            []string   `json:"tags"`
	IncludeFacts    bool       `json:"include_facts"`
	Content         string     `json:"content"`
	Metadata        model.JSON `json:"metadata"`
	MemoryID        string     `json:"memory_id"`
	Limit           int        `json:"limit"`
	HivySessionID   string     `json:"_hivy_session_id"`
	HivyActorUserID string     `json:"_hivy_actor_user_id"`
}

func (s *Service) loadOrgAgent(ctx context.Context, orgID, agentID uuid.UUID) (*model.Agent, error) {
	var agent model.Agent
	if err := s.cfg.DB.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").
		First(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

// resolveManageChannel parses the channel_id arg. explicit reports whether the
// arg was supplied at all: an omitted arg means "all channels" for reads and
// "org-wide" for writes. "org" (or a nil id) selects organization-wide memories.
func resolveManageChannel(raw string) (channelID *uuid.UUID, explicit bool, err error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, false, nil
	}
	if strings.EqualFold(text, "org") {
		return nil, true, nil
	}
	id, parseErr := uuid.Parse(text)
	if parseErr != nil || id == uuid.Nil {
		return nil, true, fmt.Errorf("channel_id must be a valid UUID or \"org\"")
	}
	return &id, true, nil
}

func registerManageMemoriesTool(server *mcp.Server, service *Service, token *model.Token) {
	server.AddTool(&mcp.Tool{
		Name:        manageMemoriesToolName,
		Description: manageMemoriesDescription,
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"search", "overview", "forget", "retain"},
					"description": "search runs a semantic search across all channels; overview returns aggregate counts; forget archives a memory; retain stores a memory.",
				},
				"channel_id": map[string]any{
					"type":        "string",
					"description": "UUID of a channel, or \"org\" for organization-wide memories. For search it narrows results; for retain it targets the write. Omit search to span all channels; omit retain to store organization-wide.",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Required for action search. Short semantic search phrase, max 6 words and 40 characters.",
					"maxLength":   memoryToolQueryMaxChars,
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Required for action retain. One durable fact, preference, rule, or decision.",
				},
				"memory_id": map[string]any{
					"type":        "string",
					"description": "Required for action forget. UUID of the observation or raw fact memory to archive.",
				},
				"tags":          memoryTagsSchema("Optional lowercase kebab-case slugs. Filter for search; labels for retain."),
				"include_facts": memoryIncludeFactsSchema(),
				"metadata": map[string]any{
					"type":        "object",
					"description": "Optional for action retain. Small JSON object of non-sensitive source metadata.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     50,
					"description": "Optional for action search. Maximum results to return, default 10, max 50.",
				},
			},
			"required": []string{"action"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args manageMemoriesArgs
		if err := decodeMemoryToolArgs(req, &args); err != nil {
			return memoryToolError(err.Error()), nil
		}
		// Managing every channel's memories is an org-wide privilege: require the
		// acting human to be an org admin/owner. Automated runs (no actor) keep the
		// default-agent gate applied at registration.
		if errResult := service.requireOrgManagerActor(ctx, token.OrgID, args.HivyActorUserID); errResult != nil {
			return errResult, nil
		}
		switch strings.ToLower(strings.TrimSpace(args.Action)) {
		case "search":
			return handleManageSearch(ctx, service, token, args)
		case "overview", "list":
			return handleManageOverview(ctx, service, token)
		case "forget":
			return handleManageForget(ctx, service, token, args)
		case "retain":
			return handleManageRetain(ctx, service, token, args)
		default:
			return memoryToolError("action must be search, overview, forget, or retain"), nil
		}
	})
}
