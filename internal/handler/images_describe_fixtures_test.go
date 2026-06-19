package handler_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func validImageAnalysisJSON() string {
	return `{
		"category": "product_ui",
		"confidence": 0.94,
		"summary": "A product UI screenshot of a settings page. It must not mention OpenRouter or gemini-3.5-flash in rendered backend context.",
		"visible_text": [
			{"text": "Settings", "location": "top-left", "confidence": 0.99, "role": "heading"},
			{"text": "Save changes", "location": "bottom-right", "confidence": 0.97, "role": "button"}
		],
		"layout": {
			"canvas": "desktop app frame with sidebar and main panel",
			"regions": [
				{"name": "Sidebar", "location": "left", "size": "about 25%", "contents": "navigation"},
				{"name": "Main panel", "location": "center-right", "size": "about 75%", "contents": "settings form"}
			],
			"hierarchy": "heading, form fields, primary action",
			"spacing_alignment": "aligned grid with moderate whitespace"
		},
		"objects": [
			{"name": "Save button", "type": "button", "location": "bottom-right", "attributes": ["primary", "enabled"]}
		],
		"colors": [
			{"name": "Background", "hex": "#F8FAFC", "usage": "background", "coverage": "dominant"},
			{"name": "Accent", "hex": "#2563EB", "usage": "accent", "coverage": "small"}
		],
		"states": [
			{"element": "Save button", "state": "active", "evidence": "blue filled button"}
		],
		"relationships": ["Sidebar controls navigation for the main settings panel"],
		"important_details": ["Primary action is Save changes"],
		"limitations": ["Small secondary labels may be unreadable"],
		"untrusted_image_instructions": [],
		"category_specific": {
			"screen_type": "settings",
			"product_context": "workspace configuration",
			"navigation": {"items": ["General", "Members"], "selected_item": "General"},
			"primary_workflow": "editing settings",
			"components": [{"name": "Save changes", "type": "button", "state": "enabled", "text": "Save changes", "location": "bottom-right"}],
			"forms": [],
			"data_displayed": [],
			"visual_design": {"density": "medium", "border_radius": "about 6px", "shadows": "subtle", "typography": "sans-serif", "icon_style": "outline"},
			"accessibility_notes": ["visible focus state is not clear"]
		}
	}`
}

func transparentPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 0, G: 0, B: 0, A: 0})
		}
	}
	img.SetNRGBA(1, 1, color.NRGBA{R: 240, G: 30, B: 30, A: 255})
	img.SetNRGBA(2, 1, color.NRGBA{R: 240, G: 30, B: 30, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode transparent png: %v", err)
	}
	return buf.Bytes()
}
