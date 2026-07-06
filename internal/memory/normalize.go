package memory

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

var tagPartRE = regexp.MustCompile(`[^a-z0-9]+`)

func NormalizeTags(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		tag := strings.ToLower(strings.TrimSpace(value))
		tag = tagPartRE.ReplaceAllString(tag, "-")
		tag = strings.Trim(tag, "-")
		if len(tag) > 64 {
			tag = strings.Trim(tag[:64], "-")
		}
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
		if len(out) >= MaxTags {
			break
		}
	}
	return out
}

func normalizeContent(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("content is required")
	}
	if len(value) > MaxContentLength {
		return "", fmt.Errorf("content must be at most %d characters", MaxContentLength)
	}
	return value, nil
}

func normalizeMetadata(value model.JSON) model.JSON {
	if value == nil {
		return model.JSON{}
	}
	return value
}
