package hindsight

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type memoryRetainToolResponse struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	BankID      string `json:"bank_id"`
	ItemsCount  int    `json:"items_count"`
	Async       bool   `json:"async"`
	OperationID string `json:"operation_id,omitempty"`
	DocumentID  string `json:"document_id"`
}

func addRetainTool(server *mcp.Server, agent *model.Agent, client *Client, db *gorm.DB, banks *BankProvisioner, bankID string, refresh MemoryRefreshFunc) {
	server.AddTool(
		&mcp.Tool{
			Name: "memory_retain",
			Description: `Store important information to long-term memory so it persists across conversations. Call this tool when:
- The user shares a fact, preference, decision, deadline, or commitment you should remember
- A significant decision is made or a problem is resolved — store the decision AND the reasoning
- You learn something new about the user, their projects, their team, or their goals
- The user corrects you or expresses a preference about how you should work — store the correction so you never repeat the mistake
- Important relationships between people, projects, or concepts are revealed
- A task outcome, milestone, or status change occurs that future conversations should know about

DO NOT store:
- Greetings, small talk, or conversational filler
- Information you have already stored (avoid duplicates)
- Temporary state or in-progress work details that will change immediately
- Exact conversation transcripts — distill into clear factual statements instead
- Anything the user explicitly asks you not to remember

Write the content as a clear, specific factual statement. Bad: "User talked about React." Good: "User's frontend stack is React with Zustand for state management, migrated from Redux in Q1 2026."`,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{
						"type":        "string",
						"description": "A clear, factual statement of what to remember. Write as a specific fact, not a conversation excerpt. Include names, dates, and specifics when available.",
					},
					"context": map[string]any{
						"type":        "string",
						"description": "Describe the nature and source of this information. This significantly improves how the memory is indexed and retrieved. Examples: 'Technical architecture discussion', 'User preference stated during product setup', 'Decision from Q2 planning meeting'. Do NOT use generic values like 'conversation' or 'chat'.",
					},
					"tags": memoryTagsSchema(true),
					"provider": map[string]any{
						"type":        "string",
						"description": "Deprecated. Use tags.provider instead.",
					},
				},
				"required": []string{"content", "tags"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var params struct {
				Content string         `json:"content"`
				Context string         `json:"context"`
				Tags    MemoryTagInput `json:"tags"`
			}
			var raw map[string]any
			if req.Params.Arguments != nil {
				_ = json.Unmarshal(req.Params.Arguments, &raw)
				_ = json.Unmarshal(req.Params.Arguments, &params)
			}
			if params.Content == "" {
				return toolError("content is required"), nil
			}
			if hasLegacyMemoryTagFields(raw) {
				return toolError("legacy memory tag fields are no longer accepted; pass scope/provider/resource_type/resource_id/memory_type inside the required tags object"), nil
			}
			validated, err := ValidateRetainTags(ctx, db, agent, params.Tags)
			if err != nil {
				return toolError(err.Error()), nil
			}
			if banks != nil && agent != nil && agent.OrgID != nil {
				if err := banks.EnsureOrgBank(ctx, *agent.OrgID); err != nil {
					return toolError("memory retain failed: " + err.Error()), nil
				}
			}

			documentID := "manual:" + agent.ID.String() + ":" + uuid.NewString()
			metadata := memoryMetadata(agent, documentID, validated.Input)
			result, err := client.Retain(ctx, bankID, &RetainRequest{
				Items: []RetainItem{{
					Content:           params.Content,
					Context:           params.Context,
					DocumentID:        documentID,
					Tags:              validated.RetainTags,
					Metadata:          metadata,
					ObservationScopes: memoryObservationScopes(validated.Input),
				}},
				Async: true,
			})
			if err != nil {
				return toolError("memory retain failed: " + err.Error()), nil
			}
			if refresh != nil {
				refresh(ctx, agent)
			}

			return toolJSON(memoryRetainResponse(bankID, documentID, result))
		},
	)
}

func memoryRetainResponse(bankID, documentID string, result *RetainResponse) memoryRetainToolResponse {
	out := memoryRetainToolResponse{
		Message:    "Memory retain has been accepted and will be processed in the background. It may take a little while before memory_recall reflects this new memory.",
		BankID:     bankID,
		DocumentID: documentID,
	}
	if result == nil {
		return out
	}
	out.Success = result.Success
	out.ItemsCount = result.ItemsCount
	out.Async = result.Async
	out.OperationID = result.OperationID
	if result.BankID != "" {
		out.BankID = result.BankID
	}
	return out
}

func upsertMemoryTag(tags []string, key, value string) []string {
	value = sanitizeMemoryTagValue(value)
	if value == "" {
		return tags
	}
	prefix := key + ":"
	out := tags[:0]
	for _, tag := range tags {
		if !strings.HasPrefix(tag, prefix) {
			out = append(out, tag)
		}
	}
	return append(out, prefix+value)
}

func addMetadataValue(metadata map[string]string, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	runes := []rune(value)
	if len(runes) > 256 {
		value = string(runes[:256])
	}
	metadata[key] = value
}

func sanitizeMemoryTagValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.' || r == ':':
			b.WriteRune(r)
		case unicode.IsSpace(r), r == '/':
			b.WriteByte('-')
		}
		if b.Len() >= 120 {
			break
		}
	}
	return strings.Trim(b.String(), "-_.:")
}
