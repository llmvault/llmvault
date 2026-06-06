package specialists

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadGlobalSpecialists(t *testing.T) {
	catalog, err := Load("../../global/specialists")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	items := catalog.List()
	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2", len(items))
	}
	def, ok := catalog.BySlug("software-engineering-specialist")
	if !ok {
		t.Fatalf("software engineering specialist not found")
	}
	if !def.AutoAttach {
		t.Fatalf("AutoAttach = false, want true")
	}
	if def.DefaultModel != "qwen3.7-max" {
		t.Fatalf("DefaultModel = %q, want qwen3.7-max", def.DefaultModel)
	}
	if !strings.Contains(def.SystemPrompt, "Software Engineering Specialist") {
		t.Fatalf("SystemPrompt does not contain migrated prompt")
	}
	if !strings.Contains(def.Description, "Delegate when the user gives a clear target and desired outcome") {
		t.Fatalf("software engineering description missing delegation criteria: %q", def.Description)
	}
	if !strings.Contains(def.Description, "ask for clarification first when the app, repo, file, error, page, integration, output, or constraints are missing") {
		t.Fatalf("software engineering description missing clarification criteria: %q", def.Description)
	}
	research, ok := catalog.BySlug("business-research-specialist")
	if !ok {
		t.Fatalf("business research specialist not found")
	}
	if !strings.Contains(research.Description, "Delegate when the research question and scope are clear") {
		t.Fatalf("business research description missing delegation criteria: %q", research.Description)
	}
	if !strings.Contains(research.Description, "ask for clarification first when the topic, geography, timeframe, source type, output format, or decision being supported is unclear") {
		t.Fatalf("business research description missing clarification criteria: %q", research.Description)
	}
}

func TestLoadRejectsDuplicateSlug(t *testing.T) {
	dir := t.TempDir()
	writeSpecialist(t, filepath.Join(dir, "one"), "duplicate")
	writeSpecialist(t, filepath.Join(dir, "two"), "duplicate")

	if _, err := Load(dir); err == nil {
		t.Fatalf("Load() error = nil, want duplicate slug error")
	}
}

func TestLoadRejectsMissingPrompt(t *testing.T) {
	dir := t.TempDir()
	specialistDir := filepath.Join(dir, "bad")
	if err := os.MkdirAll(specialistDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specialistDir, "agent.json"), []byte(`{
  "slug":"bad",
  "name":"Bad",
  "description":"Bad specialist",
  "specialist_type":"bad",
  "version":1,
  "default_model":"qwen3.7-max",
  "prompt_path":"missing.md"
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(dir); err == nil {
		t.Fatalf("Load() error = nil, want missing prompt error")
	}
}

func writeSpecialist(t *testing.T, dir string, slug string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent.json"), []byte(`{
  "slug":"`+slug+`",
  "name":"Test",
  "description":"Test specialist",
  "specialist_type":"test",
  "version":1,
  "default_model":"qwen3.7-max",
  "prompt_path":"prompt.md"
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("You are a test specialist."), 0o644); err != nil {
		t.Fatal(err)
	}
}
