package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func (h *UploadsHandler) generationRegistry() *registry.Registry {
	if h.imageRegistry != nil {
		return h.imageRegistry
	}
	return registry.Global()
}

func (h *UploadsHandler) resolveGenerationModel(ctx context.Context, agent *model.Agent, vector bool, sessionID *uuid.UUID) (string, error) {
	if sessionID == nil {
		modelID, _ := resolveImageModelPreference(nil, nil, agent, vector)
		return modelID, h.generationRegistry().ValidateImageGenerationModel(modelID, vector)
	}
	var session model.Session
	if err := h.db.WithContext(ctx).
		Where("id = ? AND agent_id = ? AND org_id = ?", *sessionID, agent.ID, *agent.OrgID).
		First(&session).Error; err != nil {
		return "", err
	}
	var channel model.Channel
	err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ?", session.ChannelID, *agent.OrgID).
		First(&channel).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return "", err
	}
	if err == gorm.ErrRecordNotFound {
		modelID, _ := resolveImageModelPreference(&session, nil, agent, vector)
		return modelID, h.generationRegistry().ValidateImageGenerationModel(modelID, vector)
	}
	modelID, _ := resolveImageModelPreference(&session, &channel, agent, vector)
	return modelID, h.generationRegistry().ValidateImageGenerationModel(modelID, vector)
}

func (h *UploadsHandler) imageReferenceInputs(ctx context.Context, agent *model.Agent, rawIDs []string) ([]imageGenerationReference, error) {
	if len(rawIDs) == 0 {
		return nil, nil
	}
	if len(rawIDs) > 10 {
		return nil, fmt.Errorf("reference_asset_ids must contain at most 10 assets")
	}
	ids := make([]uuid.UUID, 0, len(rawIDs))
	for _, raw := range rawIDs {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("invalid reference_asset_id %q", raw)
		}
		ids = append(ids, id)
	}
	var assets []model.AgentAsset
	if err := h.db.WithContext(ctx).
		Where("id IN ? AND org_id = ? AND agent_id = ?", ids, *agent.OrgID, agent.ID).
		Find(&assets).Error; err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]model.AgentAsset, len(assets))
	for _, asset := range assets {
		byID[asset.ID] = asset
	}
	out := make([]imageGenerationReference, 0, len(ids))
	for _, id := range ids {
		asset, ok := byID[id]
		if !ok {
			return nil, gorm.ErrRecordNotFound
		}
		url, err := h.referenceAssetURL(ctx, asset)
		if err != nil {
			return nil, err
		}
		out = append(out, imageGenerationReference{URL: url})
	}
	return out, nil
}

func (h *UploadsHandler) referenceAssetURL(ctx context.Context, asset model.AgentAsset) (string, error) {
	reader := h.AssetReader()
	if reader != nil && strings.TrimSpace(asset.Key) != "" {
		rc, err := reader.Open(ctx, asset.Key)
		if err == nil {
			defer rc.Close()
			data, err := io.ReadAll(io.LimitReader(rc, 12*1024*1024))
			if err != nil {
				return "", fmt.Errorf("read reference asset: %w", err)
			}
			if len(data) > 0 {
				return "data:" + asset.ContentType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
			}
		}
	}
	value := h.publicAssetURL(asset.Key, asset.PublicURL)
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("reference asset has no readable URL")
	}
	return value, nil
}

func (h *UploadsHandler) storeGeneratedImages(ctx context.Context, agent *model.Agent, sandbox *model.Sandbox, vector bool, providerID, modelID, upstreamModel, prompt string, referenceIDs []string, images []generatedImageBytes) ([]imageGenerationToolResult, error) {
	results := make([]imageGenerationToolResult, 0, len(images))
	for _, image := range images {
		contentType := detectGeneratedImageContentType(image.Data, vector, image.ContentType)
		filename := generatedImageFilename(vector, contentType)
		folder := "generated/images"
		if vector {
			folder = "generated/vectors"
		}
		key := buildAgentAssetKey(agent.ID, folder, filename)
		stored, err := h.streamer.Stream(ctx, key, contentType, bytes.NewReader(image.Data))
		if err != nil {
			return nil, fmt.Errorf("stream generated image: %w", err)
		}
		if stored.Bytes == 0 {
			return nil, fmt.Errorf("generated image is empty")
		}
		publicURL := h.publicAssetURL(stored.Key, "")
		description, err := generatedImageDescription(vector, providerID, modelID, upstreamModel, prompt, referenceIDs, image.Metadata)
		if err != nil {
			return nil, err
		}
		asset := model.AgentAsset{
			AgentID:     agent.ID,
			OrgID:       *agent.OrgID,
			SandboxID:   sandbox.ID,
			Path:        folder,
			Filename:    filename,
			Key:         stored.Key,
			PublicURL:   publicURL,
			ContentType: contentType,
			Bytes:       stored.Bytes,
			Description: &description,
		}
		if err := h.db.WithContext(ctx).Create(&asset).Error; err != nil {
			return nil, fmt.Errorf("persist generated image asset: %w", err)
		}
		results = append(results, imageGenerationToolResult{
			DriveAssetID:      asset.ID.String(),
			ContentType:       asset.ContentType,
			Bytes:             asset.Bytes,
			PublicURL:         h.publicAssetURL(asset.Key, asset.PublicURL),
			ReferenceAssetIDs: cleanReferenceAssetIDs(referenceIDs),
		})
	}
	return results, nil
}

func detectGeneratedImageContentType(data []byte, vector bool, declaredContentType ...string) string {
	for _, value := range declaredContentType {
		value = strings.TrimSpace(value)
		if strings.HasPrefix(strings.ToLower(value), "image/") {
			return value
		}
	}
	prefix := strings.TrimSpace(string(data[:min(len(data), 512)]))
	if strings.Contains(strings.ToLower(prefix), "<svg") {
		return "image/svg+xml"
	}
	detected := http.DetectContentType(data)
	if strings.HasPrefix(detected, "image/") {
		return detected
	}
	if vector {
		return "image/svg+xml"
	}
	return "image/png"
}

func generatedImageFilename(vector bool, contentType string) string {
	ext := extensionForImageContentType(contentType)
	stem := "ai-image"
	if vector {
		stem = "ai-vector"
	}
	return fmt.Sprintf("%s-%d%s", stem, time.Now().UnixNano(), ext)
}

func extensionForImageContentType(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/svg+xml":
		return ".svg"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		ext := filepath.Ext(contentType)
		if ext != "" {
			return ext
		}
		return ".png"
	}
}

func generatedImageDescription(vector bool, providerID, modelID, upstreamModel, prompt string, referenceIDs []string, extra model.JSON) (model.RawJSON, error) {
	meta := map[string]any{
		"auto_generated":      true,
		"generated_by":        "ai",
		"source":              "image_generation",
		"mode":                map[bool]string{true: "vector", false: "raster"}[vector],
		"provider_id":         providerID,
		"model":               modelID,
		"upstream_model":      upstreamModel,
		"prompt":              prompt,
		"reference_asset_ids": cleanReferenceAssetIDs(referenceIDs),
	}
	for key, value := range extra {
		if strings.TrimSpace(key) != "" {
			meta[key] = value
		}
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	return model.RawJSON(raw), nil
}

func cleanReferenceAssetIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if value := strings.TrimSpace(id); value != "" {
			out = append(out, value)
		}
	}
	return out
}
