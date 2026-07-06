package notion

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// NotionConfig is the connector-specific configuration stored in the
// RAGSource config blob. It is the scope contract the admin UI writes
// and the connector reads.
//
//   - RootPageIDs holds the selected page/database UUIDs. Empty means
//     "index the whole workspace"; non-empty switches the connector into
//     subtree mode rooted at exactly those pages.
//   - IncludeDatabases toggles whether inline databases (their row text
//     plus each row page as its own document) are indexed. Defaults true.
type NotionConfig struct {
	RootPageIDs      []string `json:"root_page_ids,omitempty"`
	IncludeDatabases bool     `json:"include_databases"`
}

// rawConfig mirrors the two accepted on-disk shapes for the selected
// roots. The admin UI writes a structured `scope.items[]` envelope; a
// flat `root_page_ids` array is accepted as a fallback so hand-written
// or migrated configs keep working.
type rawConfig struct {
	Scope *struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	} `json:"scope"`
	RootPageIDs      []string `json:"root_page_ids"`
	IncludeDatabases *bool    `json:"include_databases"`
}

// LoadConfig parses the raw config blob into a NotionConfig. Root ids
// are read from `scope.items[].id` when present, otherwise from the
// top-level `root_page_ids`. Every id is normalised to canonical UUID
// form, de-duplicated, and emptied entries dropped. IncludeDatabases
// defaults to true when the field is absent.
func LoadConfig(raw json.RawMessage) (NotionConfig, error) {
	cfg := NotionConfig{IncludeDatabases: true}
	if len(raw) == 0 || string(raw) == "null" {
		return cfg, nil
	}

	var rc rawConfig
	if err := json.Unmarshal(raw, &rc); err != nil {
		return NotionConfig{}, fmt.Errorf("notion: parse config: %w", err)
	}

	var ids []string
	if rc.Scope != nil {
		for _, item := range rc.Scope.Items {
			if strings.TrimSpace(item.ID) != "" {
				ids = append(ids, item.ID)
			}
		}
	}
	if len(ids) == 0 {
		ids = rc.RootPageIDs
	}
	cfg.RootPageIDs = normaliseIDList(ids)

	if rc.IncludeDatabases != nil {
		cfg.IncludeDatabases = *rc.IncludeDatabases
	}
	return cfg, nil
}

// normaliseIDList canonicalises, de-duplicates, and drops empty ids.
func normaliseIDList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, entry := range in {
		id := normalizeNotionID(entry)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

var hex32Re = regexp.MustCompile(`[0-9a-fA-F]{32}`)

// normalizeNotionID coerces a user-supplied identifier into a canonical
// 8-4-4-4-12 lowercase UUID. It accepts a bare 32-hex string, a dashed
// UUID, or a full Notion URL (where the id is the trailing 32-hex token
// of the last path segment). Inputs that contain no 32-hex run are
// returned trimmed but otherwise unchanged, so non-UUID identifiers
// pass through untouched.
func normalizeNotionID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	candidate := s
	if strings.Contains(candidate, "://") || strings.Contains(candidate, "notion.") {
		// URL form: keep the last path segment, drop any query/fragment,
		// then take the trailing token after the final dash (Notion slugs
		// look like "Human-Readable-Title-<32hex>").
		if i := strings.LastIndex(candidate, "/"); i >= 0 {
			candidate = candidate[i+1:]
		}
		if i := strings.IndexAny(candidate, "?#"); i >= 0 {
			candidate = candidate[:i]
		}
		if i := strings.LastIndex(candidate, "-"); i >= 0 {
			candidate = candidate[i+1:]
		}
	}

	cleaned := strings.ReplaceAll(candidate, "-", "")
	if len(cleaned) == 32 && isHex(cleaned) {
		return canonicalUUID(cleaned)
	}

	// Fallback: search the whole (dash-stripped) input for any 32-hex run.
	if m := hex32Re.FindString(strings.ReplaceAll(s, "-", "")); m != "" {
		return canonicalUUID(m)
	}

	return s
}

func isHex(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

func canonicalUUID(hex32 string) string {
	h := strings.ToLower(hex32)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}
