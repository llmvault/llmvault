package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/quiver"
)

const quiverUSDPerCredit = 0.01

func (h *UploadsHandler) callQuiverImages(ctx context.Context, cred *model.Credential, apiKey, upstreamModel, prompt, _ string, count int, references []imageGenerationReference, vectorize bool) ([]generatedImageBytes, imageGenerationUsage, error) {
	refs, err := quiverImageReferences(references)
	if err != nil {
		return nil, imageGenerationUsage{}, err
	}
	client := quiver.NewClient(h.imageHTTPClient, imageGenerationTimeout)

	var result quiver.Result
	if vectorize {
		if len(refs) == 0 {
			return nil, imageGenerationUsage{}, fmt.Errorf("vectorization requires a source image reference")
		}
		result, err = client.VectorizeSVG(ctx, quiver.VectorizeRequest{
			APIKey:     apiKey,
			BaseURL:    cred.BaseURL,
			AuthScheme: cred.AuthScheme,
			Model:      upstreamModel,
			Image:      refs[0],
		})
	} else {
		result, err = client.GenerateSVG(ctx, quiver.GenerateRequest{
			APIKey:     apiKey,
			BaseURL:    cred.BaseURL,
			AuthScheme: cred.AuthScheme,
			Model:      upstreamModel,
			Prompt:     prompt,
			N:          count,
			References: refs,
		})
	}
	if err != nil {
		return nil, imageGenerationUsage{}, err
	}
	if len(result.SVGs) == 0 {
		return nil, imageGenerationUsage{}, fmt.Errorf("quiver returned no svg output")
	}
	usage := imageGenerationUsage{CreditsUsed: result.Credits}
	if strings.TrimSpace(result.RequestID) != "" {
		usage.RequestIDs = append(usage.RequestIDs, result.RequestID)
	}
	metadata := quiverImageMetadata(result)
	out := make([]generatedImageBytes, 0, len(result.SVGs))
	for _, svg := range result.SVGs {
		if strings.TrimSpace(svg.Markup) == "" {
			continue
		}
		out = append(out, generatedImageBytes{
			Data:        []byte(svg.Markup),
			ContentType: svg.MimeType,
			Metadata:    metadata,
		})
	}
	if len(out) == 0 {
		return nil, imageGenerationUsage{}, fmt.Errorf("quiver returned an empty svg")
	}
	usage.Cost = imageGenerationCreditCostUSD(usage.CreditsUsed, quiverUSDPerCredit)
	return out, usage, nil
}

func quiverImageReferences(references []imageGenerationReference) ([]quiver.Reference, error) {
	if len(references) == 0 {
		return nil, nil
	}
	out := make([]quiver.Reference, 0, len(references))
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
			out = append(out, quiver.Reference{Base64Data: data})
			continue
		}
		out = append(out, quiver.Reference{URL: url})
	}
	return out, nil
}

func quiverImageMetadata(result quiver.Result) model.JSON {
	meta := model.JSON{}
	if strings.TrimSpace(result.RequestID) != "" {
		meta["request_id"] = result.RequestID
	}
	if strings.TrimSpace(result.ResponseID) != "" {
		meta["quiver_response_id"] = result.ResponseID
	}
	if result.Credits > 0 {
		meta["quiver_credits_used"] = result.Credits
	}
	return meta
}
