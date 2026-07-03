package model

import (
	"encoding/json"
	"testing"
)

func TestAutoLoadSkillsUnmarshalAcceptsShorthandAndObject(t *testing.T) {
	var list AutoLoadSkills
	raw := `["qa-registry", {"name": "browser", "files": ["references/commands.md"]}]`
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	if list[0].Name != "qa-registry" || len(list[0].Files) != 0 {
		t.Fatalf("shorthand entry = %#v, want name qa-registry with no files", list[0])
	}
	if list[1].Name != "browser" || len(list[1].Files) != 1 || list[1].Files[0] != "references/commands.md" {
		t.Fatalf("object entry = %#v", list[1])
	}
}

func TestAutoLoadSkillMarshalAlwaysEmitsFilesArray(t *testing.T) {
	b, err := json.Marshal(AutoLoadSkill{Name: "qa-registry"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got, want := string(b), `{"name":"qa-registry","files":[]}`; got != want {
		t.Fatalf("marshal = %s, want %s", got, want)
	}
}

func TestNormalizeAutoLoadSkills(t *testing.T) {
	t.Run("normalizes shorthand and trims", func(t *testing.T) {
		got, err := NormalizeAutoLoadSkills(AutoLoadSkills{
			{Name: " qa-registry "},
			{Name: "browser", Files: []string{" references/commands.md ", ""}},
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(got) != 2 || got[0].Name != "qa-registry" {
			t.Fatalf("got = %#v", got)
		}
		if got[0].Files == nil {
			t.Fatalf("files must be non-nil, got nil for %#v", got[0])
		}
		if len(got[1].Files) != 1 || got[1].Files[0] != "references/commands.md" {
			t.Fatalf("files = %#v", got[1].Files)
		}
	})

	t.Run("merges duplicate names", func(t *testing.T) {
		got, err := NormalizeAutoLoadSkills(AutoLoadSkills{
			{Name: "browser", Files: []string{"references/a.md"}},
			{Name: "browser", Files: []string{"references/b.md", "references/a.md"}},
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(got) != 1 || len(got[0].Files) != 2 {
			t.Fatalf("expected merged single entry with 2 files, got %#v", got)
		}
	})

	t.Run("rejects empty name", func(t *testing.T) {
		if _, err := NormalizeAutoLoadSkills(AutoLoadSkills{{Name: "  "}}); err == nil {
			t.Fatal("expected error for empty name")
		}
	})

	t.Run("rejects traversal and absolute paths", func(t *testing.T) {
		for _, bad := range []string{"../secrets.md", "references/../../etc/passwd", "/etc/passwd"} {
			if _, err := NormalizeAutoLoadSkills(AutoLoadSkills{{Name: "browser", Files: []string{bad}}}); err == nil {
				t.Fatalf("expected error for bad file %q", bad)
			}
		}
	})
}
