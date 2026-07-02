package reve

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

type createPayload struct {
	Prompt         string                 `json:"prompt"`
	Version        string                 `json:"version"`
	Layout         any                    `json:"layout,omitempty"`
	AspectRatio    string                 `json:"aspect_ratio,omitempty"`
	Postprocessing []PostprocessOperation `json:"postprocessing,omitempty"`
}

type remixPayload struct {
	Prompt          string   `json:"prompt"`
	Version         string   `json:"version"`
	AspectRatio     string   `json:"aspect_ratio,omitempty"`
	ReferenceImages []string `json:"reference_images"`
}

func buildCreatePayload(req GenerateRequest) (createPayload, ImageFormat, error) {
	instruction, aspectRatio, format, err := validateGenerateRequest(req)
	if err != nil {
		return createPayload{}, "", err
	}
	return createPayload{
		Prompt:         instruction,
		Version:        "latest",
		Layout:         req.Layout,
		AspectRatio:    aspectRatio,
		Postprocessing: req.Postprocessing,
	}, format, nil
}

func buildRemixPayload(req GenerateRequest) (remixPayload, ImageFormat, error) {
	instruction, aspectRatio, format, err := validateGenerateRequest(req)
	if err != nil {
		return remixPayload{}, "", err
	}
	if len(req.Postprocessing) > 0 || req.Layout != nil {
		return remixPayload{}, "", errors.New("reve remix does not support postprocessing/layout")
	}
	referenceImages, err := buildReferenceImages(req.References)
	if err != nil {
		return remixPayload{}, "", err
	}
	return remixPayload{
		Prompt:          instruction,
		Version:         "latest",
		AspectRatio:     aspectRatio,
		ReferenceImages: referenceImages,
	}, format, nil
}

func validateGenerateRequest(req GenerateRequest) (instruction, aspectRatio string, format ImageFormat, err error) {
	if strings.TrimSpace(req.APIKey) == "" {
		return "", "", "", errors.New("reve api key is required")
	}
	instruction = strings.TrimSpace(req.Instruction)
	if instruction == "" {
		return "", "", "", errors.New("reve prompt is required")
	}
	if len([]rune(instruction)) > maxInstruction {
		return "", "", "", fmt.Errorf("reve prompt exceeds %d characters", maxInstruction)
	}
	format = req.Format
	if format == "" {
		format = FormatPNG
	}
	if !validFormat(format) {
		return "", "", "", fmt.Errorf("unsupported reve image format %q", format)
	}
	aspectRatio = strings.TrimSpace(string(req.AspectRatio))
	return instruction, aspectRatio, format, nil
}

func buildReferenceImages(references []Reference) ([]string, error) {
	out := make([]string, 0, len(references))
	for i, ref := range references {
		image, err := buildReferenceImage(ref)
		if err != nil {
			return nil, fmt.Errorf("reve reference %d: %w", i, err)
		}
		out = append(out, image)
	}
	return out, nil
}

func buildReferenceImage(ref Reference) (string, error) {
	if strings.TrimSpace(ref.Ref) != "" {
		return "", errors.New("reve remix requires image data references, got url reference")
	}
	if len(ref.Data) > 0 {
		return base64.StdEncoding.EncodeToString(ref.Data), nil
	}
	data := strings.TrimSpace(ref.Base64Data)
	if data == "" {
		return "", errors.New("image data is required")
	}
	if _, err := base64.StdEncoding.DecodeString(data); err != nil {
		return "", fmt.Errorf("invalid base64 image data: %w", err)
	}
	return data, nil
}
