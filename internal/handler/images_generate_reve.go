package handler

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/reve"
)

func (h *UploadsHandler) callReveImages(ctx context.Context, cred *model.Credential, apiKey, _ string, prompt, aspectRatio string, count int, references []imageGenerationReference) ([]generatedImageBytes, imageGenerationUsage, error) {
	refs, err := reveImageReferences(references)
	if err != nil {
		return nil, imageGenerationUsage{}, err
	}
	client := reve.NewClient(h.imageHTTPClient, imageGenerationTimeout)
	out := make([]generatedImageBytes, 0, count)
	usage := imageGenerationUsage{}
	for i := 0; i < count; i++ {
		result, err := client.GenerateImage(ctx, reve.GenerateRequest{
			APIKey:      apiKey,
			BaseURL:     cred.BaseURL,
			Instruction: prompt,
			AspectRatio: reve.AspectRatio(defaultString(strings.TrimSpace(aspectRatio), string(reve.AspectAuto))),
			Format:      reve.FormatPNG,
			References:  refs,
		})
		if err != nil {
			return nil, imageGenerationUsage{}, err
		}
		if result.ContentViolation {
			usage.ContentViolation = true
			return nil, usage, fmt.Errorf("reve image generation returned content violation")
		}
		if len(result.Data) == 0 {
			return nil, imageGenerationUsage{}, fmt.Errorf("reve returned an empty image")
		}
		usage.CreditsUsed += result.CreditsUsed
		if result.CreditsRemaining > 0 {
			usage.CreditsRemaining = result.CreditsRemaining
		}
		if strings.TrimSpace(result.RequestID) != "" {
			usage.RequestIDs = append(usage.RequestIDs, result.RequestID)
		}
		out = append(out, generatedImageBytes{
			Data:        result.Data,
			ContentType: result.ContentType,
			Metadata:    reveImageMetadata(result),
		})
	}
	return out, usage, nil
}

func reveImageReferences(references []imageGenerationReference) ([]reve.Reference, error) {
	if len(references) == 0 {
		return nil, nil
	}
	out := make([]reve.Reference, 0, len(references))
	for _, ref := range references {
		url := strings.TrimSpace(ref.URL)
		if url == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(url), "data:") {
			data, err := dataURLBase64Payload(url)
			if err != nil {
				return nil, err
			}
			out = append(out, reve.Reference{Base64Data: data})
			continue
		}
		out = append(out, reve.Reference{Ref: url})
	}
	return out, nil
}

func dataURLBase64Payload(raw string) (string, error) {
	comma := strings.Index(raw, ",")
	if comma < 0 {
		return "", fmt.Errorf("invalid reference data URL")
	}
	prefix := strings.ToLower(raw[:comma])
	payload := strings.TrimSpace(raw[comma+1:])
	if payload == "" {
		return "", fmt.Errorf("empty reference data URL")
	}
	if strings.Contains(prefix, ";base64") {
		if _, err := base64.StdEncoding.DecodeString(payload); err != nil {
			return "", fmt.Errorf("invalid reference data URL base64: %w", err)
		}
		return payload, nil
	}
	return base64.StdEncoding.EncodeToString([]byte(payload)), nil
}

func reveImageMetadata(result reve.ImageResult) model.JSON {
	meta := model.JSON{}
	if strings.TrimSpace(result.RequestID) != "" {
		meta["request_id"] = result.RequestID
	}
	if strings.TrimSpace(result.Version) != "" {
		meta["reve_version"] = result.Version
	}
	if result.CreditsUsed > 0 {
		meta["reve_credits_used"] = result.CreditsUsed
	}
	if result.CreditsRemaining > 0 {
		meta["reve_credits_remaining"] = result.CreditsRemaining
	}
	return meta
}
