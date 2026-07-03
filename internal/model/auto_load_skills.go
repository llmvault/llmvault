package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// AutoLoadSkill is one skill the runtime preloads into a session before the
// first model call: it invokes skill_view for the skill root and once per
// Files entry (skill-relative linked paths like "references/commands.md") so
// the model starts with the content already in context. An empty Files loads
// just the skill root.
type AutoLoadSkill struct {
	Name  string   `json:"name"`
	Files []string `json:"files"`
}

// UnmarshalJSON accepts either the string shorthand ("slug", which loads just
// the skill root) or the normalized object form {"name": ..., "files": [...]}.
// Every stored/emitted value is normalized to the object form.
func (s *AutoLoadSkill) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var name string
		if err := json.Unmarshal(data, &name); err != nil {
			return err
		}
		s.Name = name
		s.Files = nil
		return nil
	}
	type alias AutoLoadSkill
	var raw alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = AutoLoadSkill(raw)
	return nil
}

// MarshalJSON always emits files as an array (never null) so the compiled
// AgentDefinition and stored rows carry the normalized object form.
func (s AutoLoadSkill) MarshalJSON() ([]byte, error) {
	files := s.Files
	if files == nil {
		files = []string{}
	}
	return json.Marshal(struct {
		Name  string   `json:"name"`
		Files []string `json:"files"`
	}{Name: s.Name, Files: files})
}

// AutoLoadSkills is the jsonb column type that stores the normalized auto-load
// skill list on agents and agent_catalog rows.
type AutoLoadSkills []AutoLoadSkill

func (a AutoLoadSkills) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(a)
	if err != nil {
		return nil, fmt.Errorf("marshaling AutoLoadSkills: %w", err)
	}
	return sanitizeJSONBValue(b)
}

func (a *AutoLoadSkills) Scan(value any) error {
	if value == nil {
		*a = AutoLoadSkills{}
		return nil
	}
	var raw []byte
	switch v := value.(type) {
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return fmt.Errorf("unsupported type for AutoLoadSkills: %T", value)
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		*a = AutoLoadSkills{}
		return nil
	}
	return json.Unmarshal(raw, a)
}

// NormalizeAutoLoadSkills validates and normalizes auto-load skill entries: it
// trims each name (which must be non-empty) and validates that every file is a
// relative path with no traversal (no "..") or absolute segment. Duplicate
// skill names are merged (their files unioned). It returns a non-nil normalized
// list or a descriptive error. This is the shared validator used by the catalog
// manifest loader, the agents CRUD API, and compile-time normalization.
func NormalizeAutoLoadSkills(entries []AutoLoadSkill) (AutoLoadSkills, error) {
	out := make(AutoLoadSkills, 0, len(entries))
	index := map[string]int{}
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			return nil, fmt.Errorf("auto_load_skills: skill name must not be empty")
		}
		files := make([]string, 0, len(entry.Files))
		seenFile := map[string]bool{}
		for _, file := range entry.Files {
			file = strings.TrimSpace(file)
			if file == "" {
				continue
			}
			if !validAutoLoadSkillFile(file) {
				return nil, fmt.Errorf("auto_load_skills: skill %q has invalid file %q (must be a relative path with no \"..\")", name, file)
			}
			if seenFile[file] {
				continue
			}
			seenFile[file] = true
			files = append(files, file)
		}
		if i, ok := index[name]; ok {
			for _, file := range files {
				if !containsString(out[i].Files, file) {
					out[i].Files = append(out[i].Files, file)
				}
			}
			continue
		}
		index[name] = len(out)
		out = append(out, AutoLoadSkill{Name: name, Files: files})
	}
	return out, nil
}

func validAutoLoadSkillFile(path string) bool {
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\") {
		return false
	}
	if filepath.IsAbs(path) {
		return false
	}
	for _, segment := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' }) {
		if segment == ".." {
			return false
		}
	}
	return true
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
