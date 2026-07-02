package quiver

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const maxOutputs = 16

type referenceInput struct {
	URL    string `json:"url,omitempty"`
	Base64 string `json:"base64,omitempty"`
}

type generatePayload struct {
	Model        string           `json:"model"`
	Prompt       string           `json:"prompt"`
	Instructions string           `json:"instructions,omitempty"`
	N            int              `json:"n,omitempty"`
	Stream       bool             `json:"stream"`
	References   []referenceInput `json:"references,omitempty"`
}

type vectorizePayload struct {
	Model  string         `json:"model"`
	Image  referenceInput `json:"image"`
	Stream bool           `json:"stream"`
}

func buildGeneratePayload(req GenerateRequest, prompt string) (generatePayload, error) {
	refs, err := buildReferenceInputs(req.References)
	if err != nil {
		return generatePayload{}, err
	}
	return generatePayload{
		Model:        modelOrDefault(req.Model),
		Prompt:       prompt,
		Instructions: strings.TrimSpace(req.Instructions),
		N:            normalizeN(req.N),
		Stream:       false,
		References:   refs,
	}, nil
}

func buildVectorizePayload(req VectorizeRequest) (vectorizePayload, error) {
	image, err := buildReferenceInput(req.Image)
	if err != nil {
		return vectorizePayload{}, fmt.Errorf("quiver vectorize image: %w", err)
	}
	if image == (referenceInput{}) {
		return vectorizePayload{}, errors.New("quiver vectorize requires an image url or base64 data")
	}
	return vectorizePayload{
		Model:  modelOrDefault(req.Model),
		Image:  image,
		Stream: false,
	}, nil
}

func modelOrDefault(model string) string {
	if trimmed := strings.TrimSpace(model); trimmed != "" {
		return trimmed
	}
	return DefaultModel
}

// normalizeN clamps the requested output count into Quiver's 1-16 range,
// returning 0 for a single output so the field is omitted from the payload.
func normalizeN(n int) int {
	if n <= 1 {
		return 0
	}
	if n > maxOutputs {
		return maxOutputs
	}
	return n
}

func buildReferenceInputs(references []Reference) ([]referenceInput, error) {
	if len(references) == 0 {
		return nil, nil
	}
	out := make([]referenceInput, 0, len(references))
	for i, ref := range references {
		input, err := buildReferenceInput(ref)
		if err != nil {
			return nil, fmt.Errorf("quiver reference %d: %w", i, err)
		}
		if input == (referenceInput{}) {
			continue
		}
		out = append(out, input)
	}
	return out, nil
}

func buildReferenceInput(ref Reference) (referenceInput, error) {
	if data := strings.TrimSpace(ref.Base64Data); data != "" {
		if _, err := base64.StdEncoding.DecodeString(data); err != nil {
			return referenceInput{}, fmt.Errorf("invalid base64 image data: %w", err)
		}
		return referenceInput{Base64: data}, nil
	}
	if url := strings.TrimSpace(ref.URL); url != "" {
		return referenceInput{URL: url}, nil
	}
	return referenceInput{}, nil
}
