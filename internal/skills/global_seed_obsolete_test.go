package skills_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/skills"
)

func TestSeedGlobalSkills_ArchivesObsoleteDriveSkillNames(t *testing.T) {
	db := connectDB(t)
	dir := t.TempDir()
	writeGlobalSkill(t, dir, "drive", "new drive", "# Drive\n", nil)
	orgID := createOrg(t, db)
	agent := model.Agent{
		ID:    uuid.New(),
		OrgID: &orgID,
		Model: "deepseek-v4-flash",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	obsoleteNames := []string{"asset-uploads", "public-assets-uploads", "agent-public-assets-uploads", "agent-assets-uploads"}
	for _, name := range obsoleteNames {
		skill := model.Skill{
			OrgID:      nil,
			Slug:       model.GenerateSlug(name) + "-" + uuid.New().String()[:8],
			Name:       name,
			SourceType: model.SkillSourceInline,
			RepoRef:    "main",
			Status:     model.SkillStatusPublished,
		}
		if err := db.Create(&skill).Error; err != nil {
			t.Fatalf("create obsolete skill %s: %v", name, err)
		}
		if err := db.Create(&model.AgentSkill{AgentID: agent.ID, SkillID: skill.ID}).Error; err != nil {
			t.Fatalf("attach obsolete skill %s: %v", name, err)
		}
	}

	if _, err := skills.SeedGlobalSkills(context.Background(), db, dir); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var publishedOld int64
	if err := db.Model(&model.Skill{}).
		Where("org_id IS NULL AND name IN ? AND status = ?", obsoleteNames, model.SkillStatusPublished).
		Count(&publishedOld).Error; err != nil {
		t.Fatalf("count obsolete skills: %v", err)
	}
	if publishedOld != 0 {
		t.Fatalf("expected obsolete upload skills to be archived, got %d still published", publishedOld)
	}

	var obsoleteLinks int64
	if err := db.Model(&model.AgentSkill{}).
		Joins("JOIN skills ON skills.id = agent_skills.skill_id").
		Where("agent_skills.agent_id = ? AND skills.name IN ?", agent.ID, obsoleteNames).
		Count(&obsoleteLinks).Error; err != nil {
		t.Fatalf("count obsolete agent skill links: %v", err)
	}
	if obsoleteLinks != 0 {
		t.Fatalf("expected obsolete upload skill links to be detached, got %d", obsoleteLinks)
	}
}
