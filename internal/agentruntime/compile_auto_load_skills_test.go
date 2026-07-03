package agentruntime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// With a nil DB, resolvableSkillSlugs returns ok=false so compileAutoLoadSkills
// skips name validation and emits every normalized entry. This isolates the
// normalization/emission contract from DB-backed skill resolution.
func TestCompileAutoLoadSkillsEmitsNormalizedEntries(t *testing.T) {
	agent := &model.Agent{ID: uuid.New()}
	got := compileAutoLoadSkills(context.Background(), nil, agent, model.AutoLoadSkills{
		{Name: " qa-registry "},
		{Name: "browser", Files: []string{"references/commands.md"}},
	})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (%#v)", len(got), got)
	}
	if got[0].Name != "qa-registry" || got[0].Files == nil || len(got[0].Files) != 0 {
		t.Fatalf("entry[0] = %#v, want normalized qa-registry with empty files", got[0])
	}

	// The compiled AgentDefinition must serialize the normalized object form with
	// a files array on every entry (empty array when none).
	def := AgentDefinition{AutoLoadSkills: got}
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded struct {
		AutoLoadSkills []map[string]any `json:"auto_load_skills"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.AutoLoadSkills) != 2 {
		t.Fatalf("serialized auto_load_skills = %s", raw)
	}
	files0, ok := decoded.AutoLoadSkills[0]["files"].([]any)
	if !ok || len(files0) != 0 {
		t.Fatalf("entry[0].files must be an empty array: %s", raw)
	}
	files1, ok := decoded.AutoLoadSkills[1]["files"].([]any)
	if !ok || len(files1) != 1 || files1[0] != "references/commands.md" {
		t.Fatalf("entry[1].files = %v (%s)", files1, raw)
	}
}

// An empty auto_load_skills list must omit the field from the compiled
// definition entirely (omitempty).
func TestCompileAutoLoadSkillsOmittedWhenEmpty(t *testing.T) {
	agent := &model.Agent{ID: uuid.New()}
	if got := compileAutoLoadSkills(context.Background(), nil, agent, nil); got != nil {
		t.Fatalf("got = %#v, want nil", got)
	}
	raw, err := json.Marshal(AgentDefinition{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := decoded["auto_load_skills"]; present {
		t.Fatalf("auto_load_skills must be omitted when empty: %s", raw)
	}
}
