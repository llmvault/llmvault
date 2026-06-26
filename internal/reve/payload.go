package reve

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

type createPayload struct {
	Instruction    string                 `json:"instruction"`
	References     []referencePayload     `json:"references,omitempty"`
	Layout         any                    `json:"layout,omitempty"`
	AspectRatio    string                 `json:"aspect_ratio"`
	Postprocessing []PostprocessOperation `json:"postprocessing,omitempty"`
}

type referencePayload struct {
	Image  imagePayload `json:"image"`
	Prompt string       `json:"prompt,omitempty"`
}

type imagePayload struct {
	Data string `json:"data,omitempty"`
	Ref  string `json:"ref,omitempty"`
}

func buildCreatePayload(req GenerateRequest) (createPayload, ImageFormat, error) {
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		return createPayload{}, "", errors.New("reve api key is required")
	}
	instruction := strings.TrimSpace(req.Instruction)
	if instruction == "" {
		return createPayload{}, "", errors.New("reve instruction is required")
	}
	if len([]rune(instruction)) > maxInstruction {
		return createPayload{}, "", fmt.Errorf("reve instruction exceeds %d characters", maxInstruction)
	}
	format := req.Format
	if format == "" {
		format = FormatPNG
	}
	if !validFormat(format) {
		return createPayload{}, "", fmt.Errorf("unsupported reve image format %q", format)
	}
	aspectRatio := strings.TrimSpace(string(req.AspectRatio))
	if aspectRatio == "" {
		aspectRatio = string(AspectAuto)
	}
	references, err := buildReferences(req.References)
	if err != nil {
		return createPayload{}, "", err
	}
	return createPayload{
		Instruction:    instruction,
		References:     references,
		Layout:         req.Layout,
		AspectRatio:    aspectRatio,
		Postprocessing: req.Postprocessing,
	}, format, nil
}

func buildReferences(references []Reference) ([]referencePayload, error) {
	out := make([]referencePayload, 0, len(references))
	for i, ref := range references {
		image, err := buildReferenceImage(ref)
		if err != nil {
			return nil, fmt.Errorf("reve reference %d: %w", i, err)
		}
		out = append(out, referencePayload{
			Image:  image,
			Prompt: strings.TrimSpace(ref.Prompt),
		})
	}
	return out, nil
}

func buildReferenceImage(ref Reference) (imagePayload, error) {
	data := strings.TrimSpace(ref.Base64Data)
	if len(ref.Data) > 0 {
		data = base64.StdEncoding.EncodeToString(ref.Data)
	}
	idRef := strings.TrimSpace(ref.Ref)
	if data == "" && idRef == "" {
		return imagePayload{}, errors.New("image data or ref is required")
	}
	if data != "" && idRef != "" {
		return imagePayload{}, errors.New("image data and ref are mutually exclusive")
	}
	if data != "" {
		return imagePayload{Data: data}, nil
	}
	return imagePayload{Ref: idRef}, nil
}
