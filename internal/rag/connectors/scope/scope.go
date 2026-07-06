// Package scope defines the generic "what to index" selection that a RAG
// source carries in its config. A source scoped to a subset of a connection's
// content stores that selection under the config `scope` key; connectors read
// it to constrain ingestion. One connection can back many sources, each with a
// different scope.
//
// Convention: no scope (or an empty item list) means "index everything the
// connection can access". A non-empty scope means "index only these".
package scope

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Selection is one chosen resource (a repo, a Notion page, a Linear team, ...).
// ID is the provider identifier the connector indexes by; Name is a
// human-readable label kept for display/audit.
type Selection struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

// Scope is the selection envelope stored at config.scope. ResourceType names
// the kind of resource being selected (matches the provider's resource
// catalog key, e.g. "repository", "page", "team").
type Scope struct {
	ResourceType string      `json:"resource_type"`
	Items        []Selection `json:"items"`
}

// Parse reads the scope envelope out of a RAGSource config blob. present is
// false when the config carries no scope key (the index-everything default) —
// callers should treat that as "no restriction" rather than an error.
func Parse(raw json.RawMessage) (s Scope, present bool, err error) {
	if len(raw) == 0 || string(raw) == "null" {
		return Scope{}, false, nil
	}
	var wrapper struct {
		Scope *Scope `json:"scope"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return Scope{}, false, fmt.Errorf("scope: parse config: %w", err)
	}
	if wrapper.Scope == nil {
		return Scope{}, false, nil
	}
	s = *wrapper.Scope
	s.ResourceType = strings.TrimSpace(s.ResourceType)
	return s, true, nil
}

// IDs returns the selected resource identifiers, trimmed, de-duplicated, and
// with empties dropped.
func (s Scope) IDs() []string {
	seen := make(map[string]struct{}, len(s.Items))
	out := make([]string, 0, len(s.Items))
	for _, it := range s.Items {
		id := strings.TrimSpace(it.ID)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
