package linear

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/rag/connectors/scope"
)

// Config is the connector-specific configuration stored in the RAGSource
// config blob. TeamIDs holds the Linear team identifiers the connector
// ingests — for each selected team it walks all projects, initiatives
// (reached via projects), and issues.
//
// The selection is deny-by-default: an empty or absent scope resolves to
// Config{TeamIDs: nil} and the connector ingests nothing. This differs
// from the "index everything" convention some connectors use, because
// Linear has no cheap workspace-wide enumeration keyed off a single team.
type Config struct {
	TeamIDs []string `json:"team_ids,omitempty"`
}

// linearScopeResourceType is the resource-catalog key for a Linear team scope.
const linearScopeResourceType = "team"

// rawConfig mirrors the flat fallback shape accepted alongside the scope
// envelope. Hand-written or migrated configs may set `team_ids` directly.
type rawConfig struct {
	TeamIDs []string `json:"team_ids"`
}

// LoadConfig parses the raw config blob into a Config. Team ids are read
// from the scope envelope (config.scope, resource_type "team") when
// present, otherwise from the top-level `team_ids` field. Ids are trimmed,
// de-duplicated preserving order, and empties dropped.
//
// A present scope with a resource_type other than "team" is rejected. An
// empty or absent scope yields Config{TeamIDs: nil} without error.
func LoadConfig(raw json.RawMessage) (Config, error) {
	sc, present, err := scope.Parse(raw)
	if err != nil {
		return Config{}, fmt.Errorf("linear: %w", err)
	}
	if present {
		if sc.ResourceType != linearScopeResourceType {
			return Config{}, fmt.Errorf("linear: scope resource_type must be %q", linearScopeResourceType)
		}
		return Config{TeamIDs: normaliseIDList(sc.IDs())}, nil
	}

	if len(raw) == 0 || string(raw) == "null" {
		return Config{}, nil
	}
	var rc rawConfig
	if err := json.Unmarshal(raw, &rc); err != nil {
		return Config{}, fmt.Errorf("linear: parse config: %w", err)
	}
	return Config{TeamIDs: normaliseIDList(rc.TeamIDs)}, nil
}

// normaliseIDList trims, de-duplicates preserving order, and drops empties.
func normaliseIDList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, entry := range in {
		id := strings.TrimSpace(entry)
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
