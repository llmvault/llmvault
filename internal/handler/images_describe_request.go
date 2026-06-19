package handler

import (
	"encoding/json"
	"fmt"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/system"
)

func buildImageDescribeLLMRequest(modelID string, asset model.AgentAsset, assetURL, detailLevel string, imageMetadata map[string]any) *system.LLMRequest {
	userPrompt := fmt.Sprintf(`Analyze the uploaded image as structured attachment context.

Filename: %s
Content type: %s
Detail level: %s
`, asset.Filename, asset.ContentType, detailLevel)
	if len(imageMetadata) > 0 {
		if raw, err := json.MarshalIndent(imageMetadata, "", "  "); err == nil {
			userPrompt += "\nAuto-extracted image metadata from original image bytes:\n" + string(raw) + "\n"
		}
	}
	userPrompt += "\nReturn only the JSON object required by the system instructions."
	temperature := float32(0)
	return &system.LLMRequest{
		Model: modelID,
		Messages: []system.LLMMessage{
			{Role: "system", Content: imageDescriptionSystemPrompt},
			{
				Role: "user",
				Parts: []system.LLMPart{
					{Kind: system.LLMPartText, Text: userPrompt},
					{Kind: system.LLMPartMedia, ContentType: asset.ContentType, Text: assetURL},
				},
			},
		},
		MaxTokens:      imageDescribeMaxTokens,
		Temperature:    &temperature,
		ResponseFormat: system.JSONResponseSpec(),
	}
}
