package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

const driveSearchToolName = "drive_search"

// RegisterDriveMCPTools registers the read-only agent-drive catalog. The
// current agent is derived from the proxy token; callers cannot search another
// agent's files by passing an agent_id.
func (h *UploadsHandler) RegisterDriveMCPTools(server *mcp.Server, token *model.Token) {
	if h == nil || h.db == nil || server == nil || !imageToolAgentProxy(token) {
		return
	}
	agentID, err := imageToolAgentID(token)
	if err != nil {
		return
	}
	server.AddTool(&mcp.Tool{
		Name:        driveSearchToolName,
		Description: "Search files in this agent's drive. Results include asset_id, path, filename, content type, size, and timestamps. Use drive_download in the sandbox to retrieve an exact asset.",
		InputSchema: map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"path":         map[string]any{"type": "string", "description": "Exact folder label; empty selects the drive root."},
				"path_prefix":  map[string]any{"type": "string", "description": "Folder tree prefix."},
				"q":            map[string]any{"type": "string", "description": "Fuzzy search across path, filename, content type, and storage key."},
				"search":       map[string]any{"type": "string", "description": "Alias for q."},
				"extension":    map[string]any{"type": "string", "description": "Filename extension, with or without a leading dot."},
				"content_type": map[string]any{"type": "string", "description": "Content-type prefix, such as text/ or application/pdf."},
				"created_from": map[string]any{"type": "string", "description": "RFC3339 or YYYY-MM-DD lower bound."},
				"created_to":   map[string]any{"type": "string", "description": "RFC3339 or YYYY-MM-DD upper bound."},
				"updated_from": map[string]any{"type": "string", "description": "RFC3339 or YYYY-MM-DD lower bound."},
				"updated_to":   map[string]any{"type": "string", "description": "RFC3339 or YYYY-MM-DD upper bound."},
				"sort_by":      map[string]any{"type": "string", "enum": []string{"created_at", "updated_at", "filename", "bytes", "content_type", "path"}},
				"sort_dir":     map[string]any{"type": "string", "enum": []string{"asc", "desc"}},
				"limit":        map[string]any{"type": "integer", "minimum": 1, "maximum": 200, "default": 50},
				"cursor":       map[string]any{"type": "string", "description": "Pagination cursor returned by a prior search."},
			},
		},
	},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var args driveSearchArgs
			if len(req.Params.Arguments) > 0 {
				if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
					return driveToolError("invalid tool arguments"), nil
				}
			}
			page, err := h.searchAgentDrive(ctx, token.OrgID, agentID, args)
			if err != nil {
				return driveToolError(err.Error()), nil
			}
			return driveToolJSON(page)
		},
	)
}

type driveSearchArgs struct {
	Path        *string `json:"path"`
	PathPrefix  string  `json:"path_prefix"`
	Q           string  `json:"q"`
	Search      string  `json:"search"`
	Extension   string  `json:"extension"`
	ContentType string  `json:"content_type"`
	CreatedFrom string  `json:"created_from"`
	CreatedTo   string  `json:"created_to"`
	UpdatedFrom string  `json:"updated_from"`
	UpdatedTo   string  `json:"updated_to"`
	SortBy      string  `json:"sort_by"`
	SortDir     string  `json:"sort_dir"`
	Limit       int     `json:"limit"`
	Cursor      string  `json:"cursor"`
}

type driveSearchItem struct {
	ID          string `json:"asset_id"`
	Path        string `json:"path"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Bytes       int64  `json:"bytes"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type driveSearchPage struct {
	Data       []driveSearchItem `json:"data"`
	HasMore    bool              `json:"has_more"`
	NextCursor *string           `json:"next_cursor,omitempty"`
}

func (h *UploadsHandler) searchAgentDrive(ctx context.Context, orgID, agentID uuid.UUID, args driveSearchArgs) (driveSearchPage, error) {
	limit := args.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 200 {
		return driveSearchPage{}, fmt.Errorf("limit must be between 1 and 200")
	}
	q := h.db.WithContext(ctx).Table("agent_assets AS ea").Where("ea.org_id = ? AND ea.agent_id = ?", orgID, agentID)
	if args.Path != nil {
		q = q.Where("ea.path = ?", strings.Trim(strings.TrimSpace(*args.Path), "/"))
	}
	if value := strings.Trim(strings.TrimSpace(args.PathPrefix), "/"); value != "" {
		q = q.Where("ea.path = ? OR ea.path LIKE ?", value, escapeLike(value)+"/%")
	}
	needleValue := strings.TrimSpace(args.Q)
	if needleValue == "" {
		needleValue = strings.TrimSpace(args.Search)
	}
	if needleValue != "" {
		needle := "%" + escapeLike(strings.ToLower(needleValue)) + "%"
		q = q.Where("LOWER(ea.path) LIKE ? OR LOWER(ea.filename) LIKE ? OR LOWER(ea.content_type) LIKE ? OR LOWER(ea.key) LIKE ?", needle, needle, needle, needle)
	}
	if value := strings.TrimSpace(args.Extension); value != "" {
		q = q.Where("LOWER(ea.filename) LIKE ?", "%."+escapeLike(strings.TrimPrefix(strings.ToLower(value), ".")))
	}
	if value := strings.TrimSpace(args.ContentType); value != "" {
		q = q.Where("LOWER(ea.content_type) LIKE ?", escapeLike(strings.ToLower(value))+"%")
	}
	for _, filter := range []struct {
		raw, column string
		end         bool
	}{
		{args.CreatedFrom, "ea.created_at", false}, {args.CreatedTo, "ea.created_at", true},
		{args.UpdatedFrom, "ea.updated_at", false}, {args.UpdatedTo, "ea.updated_at", true},
	} {
		if strings.TrimSpace(filter.raw) == "" {
			continue
		}
		value, err := parseAssetListTime(filter.raw, filter.end)
		if err != nil {
			return driveSearchPage{}, fmt.Errorf("invalid date filter")
		}
		op := ">="
		if filter.end {
			op = "<="
		}
		q = q.Where(filter.column+" "+op+" ?", value)
	}
	sortColumn := "ea.created_at"
	switch strings.TrimSpace(args.SortBy) {
	case "", "created_at":
	case "updated_at":
		sortColumn = "ea.updated_at"
	case "filename":
		sortColumn = "ea.filename"
	case "bytes":
		sortColumn = "ea.bytes"
	case "content_type":
		sortColumn = "ea.content_type"
	case "path":
		sortColumn = "ea.path"
	default:
		return driveSearchPage{}, fmt.Errorf("invalid sort_by")
	}
	sortDir := strings.ToLower(strings.TrimSpace(args.SortDir))
	if sortDir == "" {
		sortDir = "desc"
	}
	if sortDir != "asc" && sortDir != "desc" {
		return driveSearchPage{}, fmt.Errorf("invalid sort_dir")
	}
	if args.Cursor != "" {
		if sortColumn != "ea.created_at" && sortColumn != "ea.updated_at" {
			return driveSearchPage{}, fmt.Errorf("cursor requires created_at or updated_at sorting")
		}
		n, err := strconv.ParseInt(args.Cursor, 10, 64)
		if err != nil {
			return driveSearchPage{}, fmt.Errorf("invalid cursor")
		}
		op := "<"
		if sortDir == "asc" {
			op = ">"
		}
		q = q.Where(sortColumn+" "+op+" ?", time.Unix(0, n))
	}
	var rows []model.AgentAsset
	if err := q.Order(sortColumn + " " + sortDir).Limit(limit + 1).Find(&rows).Error; err != nil {
		return driveSearchPage{}, fmt.Errorf("failed to search drive")
	}
	page := driveSearchPage{HasMore: len(rows) > limit}
	if page.HasMore {
		rows = rows[:limit]
	}
	page.Data = make([]driveSearchItem, 0, len(rows))
	for _, asset := range rows {
		page.Data = append(page.Data, driveSearchItem{ID: asset.ID.String(), Path: asset.Path, Filename: asset.Filename, ContentType: asset.ContentType, Bytes: asset.Bytes, CreatedAt: asset.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: asset.UpdatedAt.UTC().Format(time.RFC3339)})
	}
	if page.HasMore {
		last := rows[len(rows)-1]
		cursorTime := last.CreatedAt
		if sortColumn == "ea.updated_at" {
			cursorTime = last.UpdatedAt
		}
		value := strconv.FormatInt(cursorTime.UnixNano(), 10)
		page.NextCursor = &value
	}
	return page, nil
}

func driveToolError(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + message}}}
}

func driveToolJSON(value any) (*mcp.CallToolResult, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return driveToolError("failed to serialize response"), nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil
}
