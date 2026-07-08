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
// being an org owner/admin. A nil result means allowed.
//
// manage_memories exposes memory across EVERY channel plus org-wide memories —
// a cross-channel read privilege. A nil actor (automated trigger/cron run, or a
// runtime that never injected an identity) is REJECTED: an unattributed,
// possibly prompt-injected automated run must not be able to exfiltrate the
// whole org's memory through the default agent. Per-channel memory recall used
// during normal turns is a separate, channel-scoped path and is unaffected.
// rawActorUserID is the runtime-injected `_hivy_actor_user_id`.
func (s *Service) requireOrgManagerActor(ctx context.Context, orgID uuid.UUID, rawActorUserID string) *mcp.CallToolResult {
	actor, err := access.Resolve(ctx, s.cfg.DB, orgID, rawActorUserID)
	if err != nil {
		return memoryToolError(err.Error())
	}
	if actor == nil {
		return memoryToolError("Not allowed: the org-wide memory view requires an org admin or owner acting in the session. " +
			"This looks like an automated or unattributed run with no acting user, so the request is refused.")
	}
	if !actor.IsOrgManager() {
		return memoryToolError("Not allowed: viewing the organization's memories across all channels requires an admin or owner. " +
			"The person you're acting for has the role \"" + actor.OrgRole + "\", which can only see this channel's own memories. " +
			"Ask an organization admin or owner if you need the org-wide view.")
	}
	return nil
}

const manageMemoriesDescription = `Read-only org-wide memory view across ALL channels and organization-wide memories. Available only to the org's default agent.

action search runs a semantic search over consolidated observations across every channel plus organization-wide memories, with the owning channel identified per result. Pass channel_id to narrow to one channel, or channel_id "org" for organization-wide memories only. query is a 2-6 word phrase; tags are optional exact lowercase slug filters. Set include_facts true to debug the raw extracted facts layer instead.

action overview returns aggregate counts: total memories, organization-wide count, per-channel counts, and top tags.

Memory is read-only to agents: new memories are written automatically by background reflection over sessions, and corrections or deletions happen in the memories UI.`

type manageMemoriesArgs struct {
	Action          string   `json:"action"`
	ChannelID       string   `json:"channel_id"`
	Query           string   `json:"query"`
	Tags            []string `json:"tags"`
	IncludeFacts    bool     `json:"include_facts"`
	Limit           int      `json:"limit"`
	HivySessionID   string   `json:"_hivy_session_id"`
	HivyActorUserID string   `json:"_hivy_actor_user_id"`
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
// arg was supplied at all: an omitted arg means "all channels". "org" (or a
// nil id) selects organization-wide memories.
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
					"enum":        []string{"search", "overview"},
					"description": "search runs a semantic search across all channels; overview returns aggregate counts.",
				},
				"channel_id": map[string]any{
					"type":        "string",
					"description": "Optional for action search. UUID of a channel, or \"org\" for organization-wide memories only. Omit to span all channels.",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Required for action search. Short semantic search phrase, max 6 words and 40 characters.",
					"maxLength":   memoryToolQueryMaxChars,
				},
				"tags":          memoryTagsSchema("Optional lowercase kebab-case slug filters for search."),
				"include_facts": memoryIncludeFactsSchema(),
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
		// Viewing every channel's memories is an org-wide privilege: require the
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
		default:
			return memoryToolError("action must be search or overview"), nil
		}
	})
}
