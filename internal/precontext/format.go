package precontext

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode/utf8"
)

var whitespaceRE = regexp.MustCompile(`\s+`)
var secretAssignmentRE = regexp.MustCompile(`(?i)(api[_ -]?key|token|secret|password|credential)\s*[:=]\s*[^\s,;]+`)

func section(title string, body string, maxBytes int) string {
	body = trimToBytes(strings.TrimSpace(body), maxBytes-len(title)-8)
	if body == "" {
		return ""
	}
	return title + "\n" + body
}

func joinSections(sections []string, maxBytes int) string {
	parts := make([]string, 0, len(sections))
	for _, item := range sections {
		item = strings.TrimSpace(item)
		if item != "" {
			parts = append(parts, item)
		}
	}
	return trimToBytes(strings.Join(parts, "\n\n"), maxBytes)
}

func cleanText(value string) string {
	value = strings.TrimSpace(value)
	value = whitespaceRE.ReplaceAllString(value, " ")
	value = secretAssignmentRE.ReplaceAllString(value, "$1=[redacted]")
	return value
}

func trimToBytes(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	cut := maxBytes - 3
	if cut <= 0 {
		return "..."
	}
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	value = strings.TrimSpace(value[:cut])
	if value == "" {
		return ""
	}
	return value + "..."
}

func queryHash(query string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(query))))
	return hex.EncodeToString(sum[:])[:16]
}

func firstString(values ...any) string {
	for _, value := range values {
		switch v := value.(type) {
		case string:
			if out := cleanText(v); out != "" {
				return out
			}
		case map[string]any:
			if out := firstString(v["text"], v["content"], v["summary"], v["fact"], v["observation"], v["memory"]); out != "" {
				return out
			}
		}
	}
	return ""
}
