package skills

import (
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestSkillManagerEnabled(t *testing.T) {
	if skillManagerEnabled(nil) {
		t.Fatal("nil agent must not be enabled")
	}
	if !skillManagerEnabled(&model.Agent{IsDefault: true}) {
		t.Fatal("default agent must be enabled")
	}
	if skillManagerEnabled(&model.Agent{}) {
		t.Fatal("non-default agent without allow-list must not be enabled")
	}
	allowed := &model.Agent{McpToolFilter: &model.ToolFilter{Allow: []string{"create_skill"}}}
	if !skillManagerEnabled(allowed) {
		t.Fatal("allow-listed agent must be enabled")
	}
	other := &model.Agent{McpToolFilter: &model.ToolFilter{Allow: []string{"create_agent"}}}
	if skillManagerEnabled(other) {
		t.Fatal("unrelated allow-list must not enable skill manager")
	}
}

func TestValidateSkillFilePath(t *testing.T) {
	valid := []string{
		"references/api.md",
		"scripts/check.sh",
		"templates/proposal.md",
		"assets/logo.svg",
		"references/nested/deep.md",
	}
	for _, path := range valid {
		if err := validateSkillFilePath(path); err != nil {
			t.Errorf("path %q should be valid: %v", path, err)
		}
	}
	invalid := []string{
		"",
		"SKILL.md",
		"references",
		"references/",
		"/references/api.md",
		"references/../secrets.md",
		"../references/api.md",
		"docs/api.md",
		"references\\api.md",
		"references/./api.md",
	}
	for _, path := range invalid {
		if err := validateSkillFilePath(path); err == nil {
			t.Errorf("path %q should be rejected", path)
		}
	}
}

func TestValidateSkillFieldsRejectsFrontmatterAndBadEnvNames(t *testing.T) {
	if res := validateSkillFields("Name", "Use when testing.", "---\nname: x\n---\nbody", nil, nil); res == nil {
		t.Fatal("content with frontmatter must be rejected")
	}
	if res := validateSkillFields("Name", "Use when testing.", "# Body", nil, []string{"lower_case"}); res == nil {
		t.Fatal("lowercase env var name must be rejected")
	}
	if res := validateSkillFields("Name", "Use when testing.", "# Body", nil, []string{"HIVY_ORG_API_KEY"}); res != nil {
		t.Fatalf("valid fields rejected: %v", toolResultText(res))
	}
	if res := validateSkillFields("Name", "", "# Body", nil, nil); res == nil {
		t.Fatal("empty description must be rejected")
	}
}
