package notion

import (
	"fmt"
	"strings"
)

// blockText pulls the renderable text out of a block. Regular blocks
// carry a rich_text array; table_row blocks carry a cells matrix
// (list-of-lists of rich text) which is tab-joined per row.
func blockText(m map[string]any) []string {
	obj := blockTypeObj(m)
	if obj == nil {
		return nil
	}

	var parts []string
	if rich, ok := obj["rich_text"].([]any); ok {
		for _, r := range rich {
			rm, ok := r.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := rm["text"].(map[string]any); ok {
				if content, ok := text["content"].(string); ok {
					parts = append(parts, content)
				}
			}
		}
	}

	if cells, ok := obj["cells"].([]any); ok {
		rowParts := make([]string, 0, len(cells))
		for _, cell := range cells {
			cellArr, _ := cell.([]any)
			words := make([]string, 0, len(cellArr))
			for _, rt := range cellArr {
				rtm, ok := rt.(map[string]any)
				if !ok {
					continue
				}
				if pt, ok := rtm["plain_text"].(string); ok {
					words = append(words, pt)
				}
			}
			rowParts = append(rowParts, strings.Join(words, " "))
		}
		parts = append(parts, strings.Join(rowParts, "\t"))
	}

	return parts
}

// propertiesToStr renders a page/row property map to a flat "Name: value"
// string. Keys are emitted in sorted order for deterministic output.
func propertiesToStr(properties map[string]any) string {
	if len(properties) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, name := range sortedKeys(properties) {
		prop, ok := properties[name].(map[string]any)
		if !ok {
			continue
		}
		if value, ok := recurseProperty(prop); ok && value != "" {
			sb.WriteString(name)
			sb.WriteString(": ")
			sb.WriteString(value)
			sb.WriteString("\t")
		}
	}
	return sb.String()
}

// recurseProperty resolves a single property's scalar value. It descends
// through the type-tagged wrappers Notion uses, capturing a user's
// "name" (people/created_by/etc.) before drilling past it, rendering
// date ranges as "start - end", and pulling name/content out of the
// innermost object. Returns ok=false when nothing indexable is present.
func recurseProperty(v any) (string, bool) {
	cur := v
	for {
		m, ok := cur.(map[string]any)
		if !ok {
			break
		}
		typeName, ok := m["type"].(string)
		if !ok {
			break
		}
		// User objects carry "name" alongside "type": "person"/"bot".
		// Capture it before descending, but not for title properties
		// where "name" is not the display value.
		if name, ok := m["name"].(string); ok && typeName != "title" {
			return name, true
		}
		next, exists := m[typeName]
		if !exists {
			break
		}
		cur = next
		if isEmptyValue(cur) {
			return "", false
		}
	}

	switch t := cur.(type) {
	case []any:
		return recursePropertyList(t)
	case string:
		return t, t != ""
	case map[string]any:
		if name, ok := t["name"].(string); ok {
			return name, name != ""
		}
		if content, ok := t["content"].(string); ok {
			return content, content != ""
		}
		start, hasStart := t["start"].(string)
		end, hasEnd := t["end"].(string)
		if hasStart && start != "" {
			if hasEnd && end != "" {
				return start + " - " + end, true
			}
			return start, true
		}
		if hasEnd && end != "" {
			return "Until " + end, true
		}
		// A bare id reference is not useful in plaintext.
		if _, ok := t["id"]; ok {
			return "", false
		}
	}
	return "", false
}

func recursePropertyList(items []any) (string, bool) {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		switch it := item.(type) {
		case map[string]any:
			if s, ok := recurseProperty(it); ok && s != "" {
				parts = append(parts, s)
			}
		case []any:
			if s, ok := recursePropertyList(it); ok && s != "" {
				parts = append(parts, s)
			}
		default:
			if it != nil {
				parts = append(parts, fmt.Sprint(it))
			}
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, ", "), true
}

// readPageTitle extracts a page's title: the database name for a wiki,
// otherwise the plain_text of the first title-typed property. Returns ""
// when no title is present so callers can apply a fallback.
func readPageTitle(page NotionPage) string {
	if page.DatabaseName != "" {
		return page.DatabaseName
	}
	for _, key := range sortedKeys(page.Properties) {
		prop, ok := page.Properties[key].(map[string]any)
		if !ok {
			continue
		}
		if getString(prop, "type") != "title" {
			continue
		}
		titleArr, ok := prop["title"].([]any)
		if !ok || len(titleArr) == 0 {
			continue
		}
		parts := make([]string, 0, len(titleArr))
		for _, t := range titleArr {
			tm, ok := t.(map[string]any)
			if !ok {
				continue
			}
			if pt, ok := tm["plain_text"].(string); ok {
				parts = append(parts, pt)
			}
		}
		return strings.TrimSpace(strings.Join(parts, " "))
	}
	return ""
}
