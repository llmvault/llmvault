package handler

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestNormalizeAgentAutoLoadSkillsForRequest(t *testing.T) {
	t.Run("nil yields empty list ok", func(t *testing.T) {
		w := httptest.NewRecorder()
		got, ok := normalizeAgentAutoLoadSkillsForRequest(w, nil)
		if !ok || got == nil || len(got) != 0 {
			t.Fatalf("got=%#v ok=%v", got, ok)
		}
	})

	t.Run("normalizes shorthand and object entries", func(t *testing.T) {
		var entries []model.AutoLoadSkill
		if err := json.Unmarshal([]byte(`["qa-registry", {"name":"browser","files":["references/commands.md"]}]`), &entries); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		w := httptest.NewRecorder()
		got, ok := normalizeAgentAutoLoadSkillsForRequest(w, &entries)
		if !ok || len(got) != 2 || got[0].Name != "qa-registry" || got[1].Files[0] != "references/commands.md" {
			t.Fatalf("got=%#v ok=%v", got, ok)
		}
	})

	t.Run("rejects traversal path with 400", func(t *testing.T) {
		entries := []model.AutoLoadSkill{{Name: "browser", Files: []string{"../secrets.md"}}}
		w := httptest.NewRecorder()
		if _, ok := normalizeAgentAutoLoadSkillsForRequest(w, &entries); ok {
			t.Fatal("expected ok=false for traversal path")
		}
		if w.Code != 400 {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("rejects empty name with 400", func(t *testing.T) {
		entries := []model.AutoLoadSkill{{Name: "  "}}
		w := httptest.NewRecorder()
		if _, ok := normalizeAgentAutoLoadSkillsForRequest(w, &entries); ok {
			t.Fatal("expected ok=false for empty name")
		}
		if w.Code != 400 {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}

func TestNormalizeSubAgentAutoLoadSkillsForRequestPrefixesName(t *testing.T) {
	entries := []model.AutoLoadSkill{{Name: "browser", Files: []string{"/etc/passwd"}}}
	w := httptest.NewRecorder()
	if _, ok := normalizeSubAgentAutoLoadSkillsForRequest(w, "test-executor", &entries); ok {
		t.Fatal("expected ok=false for absolute path")
	}
	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got := body["error"]; got == "" || got[:len(`sub-agent "test-executor"`)] != `sub-agent "test-executor"` {
		t.Fatalf("error = %q, want sub-agent name prefix", got)
	}
}
