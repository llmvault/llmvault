package reve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ImageFormat string

const (
	FormatPNG  ImageFormat = "image/png"
	FormatJPEG ImageFormat = "image/jpeg"
	FormatWEBP ImageFormat = "image/webp"
)

type AspectRatio string

const (
	Aspect16x9 AspectRatio = "16:9"
	Aspect9x16 AspectRatio = "9:16"
	Aspect3x2  AspectRatio = "3:2"
	Aspect2x3  AspectRatio = "2:3"
	Aspect4x3  AspectRatio = "4:3"
	Aspect3x4  AspectRatio = "3:4"
	Aspect1x1  AspectRatio = "1:1"
)

type Client struct {
	httpClient *http.Client
	timeout    time.Duration
}

func NewClient(httpClient *http.Client, timeout time.Duration) *Client {
	return &Client{httpClient: httpClient, timeout: timeout}
}

type GenerateRequest struct {
	APIKey         string
	BaseURL        string
	Instruction    string
	AspectRatio    AspectRatio
	Format         ImageFormat
	References     []Reference
	Layout         any
	Postprocessing []PostprocessOperation
}

type Reference struct {
	Data       []byte
	Base64Data string
	Ref        string
	Prompt     string
}

type PostprocessOperation struct {
	Process          string         `json:"process"`
	UpscaleFactor    int            `json:"upscale_factor,omitempty"`
	MaxDim           int            `json:"max_dim,omitempty"`
	MaxWidth         int            `json:"max_width,omitempty"`
	MaxHeight        int            `json:"max_height,omitempty"`
	EffectName       string         `json:"effect_name,omitempty"`
	EffectParameters map[string]any `json:"effect_parameters,omitempty"`
}

func Upscale(factor int) PostprocessOperation {
	return PostprocessOperation{Process: "upscale", UpscaleFactor: factor}
}

func RemoveBackground() PostprocessOperation {
	return PostprocessOperation{Process: "remove_background"}
}

func FitImage(maxDim int) PostprocessOperation {
	return PostprocessOperation{Process: "fit_image", MaxDim: maxDim}
}

func Effect(name string, parameters map[string]any) PostprocessOperation {
	return PostprocessOperation{
		Process:          "effect",
		EffectName:       name,
		EffectParameters: parameters,
	}
}

type ImageResult struct {
	Data             []byte
	ContentType      string
	Layout           json.RawMessage
	Version          string
	ContentViolation bool
	RequestID        string
	CreditsUsed      int
	CreditsRemaining int
}

type StatusError struct {
	StatusCode int
	RequestID  string
	ErrorCode  string
	Message    string
	Params     map[string]any
	Body       string
}

func (e *StatusError) Error() string {
	parts := []string{fmt.Sprintf("reve image generation failed with status %d", e.StatusCode)}
	if e.ErrorCode != "" {
		parts = append(parts, "code "+e.ErrorCode)
	}
	if e.Message != "" {
		parts = append(parts, e.Message)
	} else if e.Body != "" {
		parts = append(parts, e.Body)
	}
	if e.RequestID != "" {
		parts = append(parts, "request_id "+e.RequestID)
	}
	return strings.Join(parts, ": ")
}
