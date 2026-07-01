package skills

import (
	"strings"
	"testing"

	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/model"
)

func testSkill() (model.Skill, Bundle) {
	desc := "Parse invoices"
	skill := model.Skill{
		Slug:        "invoice-parser",
		Name:        "Invoice Parser",
		Description: &desc,
		Category:    "finance",
		Tags:        pq.StringArray{"ap", "ocr"},
	}
	bundle := Bundle{
		Description: "bundle desc (fallback)",
		Content:     "# Invoice Parser\nDo the thing.",
		Files: map[string]string{
			"references/format.md": "format doc",
		},
		References: []Reference{
			{Path: "scripts/extract.py", Body: "print('hi')"},
		},
		RequiredEnvironmentVariables: []string{"OCR_API_KEY"},
	}
	return skill, bundle
}

func TestComposeSkillMarkdown(t *testing.T) {
	skill, bundle := testSkill()
	md := composeSkillMarkdown(skill, bundle)

	if !strings.HasPrefix(md, "---\n") {
		t.Fatalf("expected frontmatter, got: %q", md)
	}
	for _, want := range []string{
		"name: invoice-parser",
		`description: "Parse invoices"`, // DB description wins over bundle
		"category: finance",
		"tags: [ap, ocr]",
		"# Invoice Parser",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("composed markdown missing %q\n%s", want, md)
		}
	}
	if !strings.HasSuffix(md, "\n") {
		t.Error("composed markdown should end with newline")
	}
}

func TestMaterializePayload(t *testing.T) {
	skill, bundle := testSkill()
	payload := materializePayload(skill, bundle)

	if got := payload["root"]; got != ".skills/invoice-parser" {
		t.Fatalf("root = %v, want .skills/invoice-parser", got)
	}
	files, ok := payload["files"].(map[string]string)
	if !ok {
		t.Fatalf("files is not a map[string]string: %T", payload["files"])
	}
	if _, ok := files["SKILL.md"]; !ok {
		t.Error("materialize must include SKILL.md")
	}
	if files["references/format.md"] != "format doc" {
		t.Errorf("missing references file, got %q", files["references/format.md"])
	}
	if files["scripts/extract.py"] != "print('hi')" {
		t.Errorf("reference (scripts) not materialized, got %q", files["scripts/extract.py"])
	}
}

func TestLinkedFileGroups(t *testing.T) {
	_, bundle := testSkill()
	groups := linkedFileGroups(bundle)

	if got := groups["references"]; len(got) != 1 || got[0] != "references/format.md" {
		t.Errorf("references group = %v", got)
	}
	if got := groups["scripts"]; len(got) != 1 || got[0] != "scripts/extract.py" {
		t.Errorf("scripts group = %v", got)
	}
	// SKILL.md is not a linked file and must not appear in any group.
	for dir, files := range groups {
		for _, f := range files {
			if f == "SKILL.md" {
				t.Errorf("SKILL.md leaked into group %q", dir)
			}
		}
	}
}

func TestSkillAllowed(t *testing.T) {
	if !skillAllowed("anything", nil) {
		t.Error("nil filter should allow all")
	}
	if !skillAllowed("anything", &model.SkillFilter{Allow: nil}) {
		t.Error("nil allow-list should allow all")
	}
	filter := &model.SkillFilter{Allow: []string{"a", "b"}}
	if !skillAllowed("a", filter) {
		t.Error("a should be allowed")
	}
	if skillAllowed("c", filter) {
		t.Error("c should be denied")
	}
}
