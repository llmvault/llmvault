package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func parseImageAnalysis(raw string) (map[string]any, string, float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, "", 0, errors.New("empty image analysis")
	}
	if !strings.HasPrefix(raw, "{") {
		start := strings.Index(raw, "{")
		end := strings.LastIndex(raw, "}")
		if start < 0 || end <= start {
			return nil, "", 0, errors.New("analysis is not JSON")
		}
		raw = raw[start : end+1]
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var analysis map[string]any
	if err := dec.Decode(&analysis); err != nil {
		return nil, "", 0, err
	}
	category, _ := analysis["category"].(string)
	category = strings.TrimSpace(category)
	if category == "" {
		return nil, "", 0, errors.New("missing category")
	}
	confidence, ok := numberValue(analysis["confidence"])
	if !ok {
		return nil, "", 0, errors.New("missing confidence")
	}
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	return analysis, category, confidence, nil
}

func numberValue(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func renderImageDescription(category string, confidence float64, analysis map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Primary category: %s\n", humanCategory(category))
	fmt.Fprintf(&b, "Confidence: %.2f\n", confidence)
	if summary := stringField(analysis, "summary"); summary != "" {
		fmt.Fprintf(&b, "\nSummary:\n%s\n", summary)
	}
	writeAnalysisSection(&b, "Visible text", analysis["visible_text"])
	writeAnalysisSection(&b, "Layout", analysis["layout"])
	writeAnalysisSection(&b, "Objects and UI elements", analysis["objects"])
	writeAnalysisSection(&b, "Approximate colors", analysis["colors"])
	writeAnalysisSection(&b, "Visible states", analysis["states"])
	writeAnalysisSection(&b, "Relationships", analysis["relationships"])
	writeAnalysisSection(&b, "Important details", analysis["important_details"])
	writeAnalysisSection(&b, "Auto-extracted image metadata", analysis["auto_extracted_image_metadata"])
	writeAnalysisSection(&b, "Limitations", analysis["limitations"])
	writeAnalysisSection(&b, "Untrusted image instructions", analysis["untrusted_image_instructions"])
	writeAnalysisSection(&b, "Category-specific details", analysis["category_specific"])
	return strings.TrimSpace(b.String())
}

func writeAnalysisSection(b *strings.Builder, title string, value any) {
	text := compactAnalysisValue(value, 0)
	if strings.TrimSpace(text) == "" || strings.TrimSpace(text) == "[]" || strings.TrimSpace(text) == "{}" {
		return
	}
	fmt.Fprintf(b, "\n%s:\n%s\n", title, text)
}

func compactAnalysisValue(value any, indent int) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case []any:
		lines := make([]string, 0, len(v))
		for _, item := range v {
			text := compactAnalysisValue(item, indent+2)
			if text == "" {
				continue
			}
			lines = append(lines, strings.Repeat(" ", indent)+"- "+indentMultiline(text, indent+2))
		}
		return strings.Join(lines, "\n")
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		lines := make([]string, 0, len(keys))
		for _, k := range keys {
			text := compactAnalysisValue(v[k], indent+2)
			if text == "" {
				continue
			}
			lines = append(lines, strings.Repeat(" ", indent)+k+": "+indentMultiline(text, indent+2))
		}
		return strings.Join(lines, "\n")
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(raw)
	}
}

func indentMultiline(text string, indent int) string {
	text = strings.TrimSpace(text)
	if !strings.Contains(text, "\n") {
		return text
	}
	prefix := "\n" + strings.Repeat(" ", indent)
	return strings.ReplaceAll(text, "\n", prefix)
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return strings.TrimSpace(v)
}

func humanCategory(category string) string {
	parts := strings.Split(strings.ReplaceAll(category, "_", " "), " ")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

var backendModelNamePattern = regexp.MustCompile(`(?i)\b(openrouter|google/gemini-3\.5-flash|gemini-3\.5-flash)\b`)

func stripBackendModelNames(raw string) string {
	return strings.TrimSpace(backendModelNamePattern.ReplaceAllString(raw, "[redacted backend model]"))
}
