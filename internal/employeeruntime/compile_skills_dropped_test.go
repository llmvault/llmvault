package employeeruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// badBundle is valid jsonb (so the NOT NULL bundle column accepts it) but fails
// to unmarshal into skillBundle, exercising the "unparseable bundle" drop path.
const badBundle = `[1,2,3]`

// TestBuildSkills_AttachedSkillWithBadBundleFailsCompile verifies P2-47: an
// explicitly attached skill whose bundle does not parse must fail the compile
// rather than being silently dropped from the agent.
func TestBuildSkills_AttachedSkillWithBadBundleFailsCompile(t *testing.T) {
	db := connectCompileTestDB(t)
	org := model.Org{Name: "Skill drop-" + uuid.NewString()}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Employee{
		ID:            uuid.New(),
		OrgID:         &org.ID,
		Name:          "Aria",
		Model:         DefaultEmployeeModel,
		Tools:         model.JSON{},
		McpServers:    model.RawJSON("[]"),
		Skills:        model.JSON{},
		Integrations:  model.JSON{},
		Resources:     model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	skill := compileTestSkill("attached-bad-"+uuid.NewString(), "Attached Bad", &org.ID)
	skill.Bundle = model.RawJSON(badBundle)
	if err := db.Create(&skill).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}
	if err := db.Create(&model.EmployeeSkill{EmployeeID: agent.ID, SkillID: skill.ID}).Error; err != nil {
		t.Fatalf("attach skill: %v", err)
	}

	_, err := buildSkills(context.Background(), db, agent.ID)
	if err == nil {
		t.Fatal("expected compile to fail for attached skill with an unparseable bundle")
	}
	if !strings.Contains(err.Error(), skill.Slug) {
		t.Fatalf("error %q should name the offending skill %q", err, skill.Slug)
	}
}

// TestBuildSkills_DefaultSkillWithBadBundleIsDropped verifies P2-47: a default
// (by-name, not explicitly attached) skill with a broken bundle is dropped, not
// fatal, so a single bad seed skill cannot break every compile. A valid sibling
// default still compiles.
func TestBuildSkills_DefaultSkillWithBadBundleIsDropped(t *testing.T) {
	db := connectCompileTestDB(t)
	agentID := uuid.New() // no EmployeeSkill links -> nothing explicitly attached

	suffix := uuid.NewString()
	badName := "default-bad-" + suffix
	goodName := "default-good-" + suffix

	bad := compileTestSkill(badName, badName, nil)
	bad.Bundle = model.RawJSON(badBundle)
	good := compileTestSkill(goodName, goodName, nil)
	for _, skill := range []model.Skill{bad, good} {
		if err := db.Create(&skill).Error; err != nil {
			t.Fatalf("create skill %s: %v", skill.Slug, err)
		}
	}
	t.Cleanup(func() {
		db.Where("slug IN ?", []string{badName, goodName}).Delete(&model.Skill{})
	})

	skills, err := buildSkillsWithDefaultNames(context.Background(), db, agentID, []string{badName, goodName})
	if err != nil {
		t.Fatalf("default bad bundle must not fail compile: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("skills = %#v, want only the good default skill", skills)
	}
	if skills[0].Name != goodName {
		t.Fatalf("compiled skill = %q, want %q", skills[0].Name, goodName)
	}
}
