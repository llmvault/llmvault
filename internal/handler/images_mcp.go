package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

const (
	imageGenerationRasterToolName    = "generate_image"
	imageGenerationVectorToolName    = "generate_vector_image"
	imageGenerationRemixToolName     = "remix_image"
	imageGenerationVectorizeToolName = "vectorize_image"
)

// RegisterImageGenerationMCPTools registers image generation tools for agent proxy MCP servers.
func (h *UploadsHandler) RegisterImageGenerationMCPTools(server *mcp.Server, token *model.Token) {
	if server == nil || h == nil || h.db == nil || !imageToolAgentProxy(token) {
		return
	}
	agentID, err := imageToolAgentID(token)
	if err != nil {
		return
	}
	sandboxID, err := imageToolSandboxID(token)
	if err != nil {
		return
	}
	registerImageGenerationMCPTool(server, h, token, agentID, sandboxID, imageGenerationRasterToolName, "raster", "Generate or revise raster images and save results to the organization drive.")
	registerImageGenerationMCPTool(server, h, token, agentID, sandboxID, imageGenerationVectorToolName, "vector", "Generate or revise SVG/vector images and save results to the organization drive.")
	registerImageGenerationMCPTool(server, h, token, agentID, sandboxID, imageGenerationRemixToolName, "remix", "Generate new images guided by reference images from the organization drive, preserving the identity of what the references show (logos, characters, products). Requires reference_asset_ids. Saves results to the organization drive.")
	registerImageGenerationMCPTool(server, h, token, agentID, sandboxID, imageGenerationVectorizeToolName, "vectorize", "Convert an existing raster image (PNG/JPEG/WebP) from the organization drive into a clean, scalable SVG vector image. Requires exactly one reference_asset_id pointing at the raster image to trace. Saves the SVG result to the organization drive.")
}

func registerImageGenerationMCPTool(server *mcp.Server, h *UploadsHandler, token *model.Token, agentID, sandboxID uuid.UUID, name, mode, description string) {
	server.AddTool(&mcp.Tool{
		Name:        name,
		Description: description,
		InputSchema: imageGenerationMCPInputSchema(mode),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := decodeImageGenerationMCPArgs(req, mode)
		if err != nil {
			return imageGenerationToolError(err.Error()), nil
		}
		agent, sandbox, err := h.imageGenerationToolContext(ctx, token, agentID, sandboxID)
		if err != nil {
			return imageGenerationToolError(err.Error()), nil
		}
		results, _, genErr := h.runImageGeneration(ctx, agent, sandbox, args)
		if genErr != nil {
			return imageGenerationToolError(genErr.Error), nil
		}
		return imageGenerationToolJSON(results)
	})
}

func imageGenerationMCPInputSchema(mode string) map[string]any {
	properties := map[string]any{
		"prompt": map[string]any{
			"type":        "string",
			"description": "Image generation prompt. Required unless description is provided.",
		},
		"description": map[string]any{
			"type":        "string",
			"description": "Alternative prompt field for describing the requested image.",
		},
		"reference_asset_ids": map[string]any{
			"type":        "array",
			"description": "Optional drive asset IDs to use as image references, max 10.",
			"items":       map[string]any{"type": "string"},
			"maxItems":    10,
		},
		"aspect_ratio": map[string]any{
			"type":        "string",
			"description": "Optional aspect ratio such as 1:1, 16:9, or 9:16.",
		},
		"type": map[string]any{
			"type":        "string",
			"description": "Optional image type hint, for example logo, icon, illustration, or photo.",
		},
		"count": map[string]any{
			"type":        "integer",
			"description": "Number of images to generate, 1 to 4.",
			"minimum":     1,
			"maximum":     imageGenerationMaxCount,
		},
	}
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
	if mode == "remix" {
		delete(properties, "type")
		properties["reference_asset_ids"] = map[string]any{
			"type":        "array",
			"description": "Drive asset IDs of the reference images to remix, 1 to 10.",
			"items":       map[string]any{"type": "string"},
			"minItems":    1,
			"maxItems":    10,
		}
		schema["required"] = []string{"reference_asset_ids"}
	}
	if mode == "vectorize" {
		delete(properties, "prompt")
		delete(properties, "description")
		delete(properties, "type")
		delete(properties, "aspect_ratio")
		delete(properties, "count")
		properties["reference_asset_ids"] = map[string]any{
			"type":        "array",
			"description": "Drive asset ID of the single raster image to vectorize into an SVG.",
			"items":       map[string]any{"type": "string"},
			"minItems":    1,
			"maxItems":    1,
		}
		schema["required"] = []string{"reference_asset_ids"}
	}
	return schema
}

func decodeImageGenerationMCPArgs(req *mcp.CallToolRequest, mode string) (imageGenerationRequest, error) {
	if req == nil || req.Params.Arguments == nil {
		return imageGenerationRequest{}, fmt.Errorf("arguments are required")
	}
	var args imageGenerationRequest
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return imageGenerationRequest{}, fmt.Errorf("invalid arguments")
	}
	args.Mode = mode
	normalized, err := normalizeImageGenerationRequest(args)
	if err != nil {
		return imageGenerationRequest{}, err
	}
	if mode == "remix" && len(cleanReferenceAssetIDs(normalized.ReferenceAssetIDs)) == 0 {
		return imageGenerationRequest{}, fmt.Errorf("remix_image requires at least one reference_asset_id")
	}
	if mode == "vectorize" && len(cleanReferenceAssetIDs(normalized.ReferenceAssetIDs)) == 0 {
		return imageGenerationRequest{}, fmt.Errorf("vectorize_image requires exactly one reference_asset_id")
	}
	return normalized, nil
}

func (h *UploadsHandler) imageGenerationToolContext(ctx context.Context, token *model.Token, agentID, sandboxID uuid.UUID) (*model.Agent, *model.Sandbox, error) {
	var agent model.Agent
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, token.OrgID, "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("agent is not active in this org")
		}
		return nil, nil, fmt.Errorf("failed to load agent")
	}
	var sandbox model.Sandbox
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND agent_id = ?", sandboxID, token.OrgID, agentID).
		First(&sandbox).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("sandbox not found for this agent")
		}
		return nil, nil, fmt.Errorf("failed to load sandbox")
	}
	return &agent, &sandbox, nil
}

func imageToolAgentProxy(token *model.Token) bool {
	if token == nil || token.Meta == nil {
		return false
	}
	tokenType, _ := token.Meta[model.TokenMetaType].(string)
	return tokenType == model.TokenTypeAgentProxy
}

func imageToolAgentID(token *model.Token) (uuid.UUID, error) {
	agentIDText, _ := token.Meta[model.TokenMetaAgentID].(string)
	agentID, err := uuid.Parse(strings.TrimSpace(agentIDText))
	if err != nil || agentID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("agent proxy token is missing agent_id")
	}
	return agentID, nil
}

func imageToolSandboxID(token *model.Token) (uuid.UUID, error) {
	sandboxIDText, _ := token.Meta[model.TokenMetaSandboxID].(string)
	sandboxID, err := uuid.Parse(strings.TrimSpace(sandboxIDText))
	if err != nil || sandboxID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("agent proxy token is missing sandbox_id")
	}
	return sandboxID, nil
}

func imageGenerationToolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + msg}},
		IsError: true,
	}
}

func imageGenerationToolJSON(v any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return imageGenerationToolError("failed to serialize response"), nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil
}
