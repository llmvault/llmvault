package handler

import (
	"github.com/google/uuid"
	"github.com/usehivy/hivy/internal/model"
	"gorm.io/gorm"
	"testing"
)

func firstTeamID(t *testing.T, db *gorm.DB, orgID uuid.UUID) uuid.UUID {
	t.Helper()
	var team model.Team
	if db.Where("org_id = ?", orgID).Order("created_at ASC").First(&team).Error == nil {
		return team.ID
	}
	team = model.Team{ID: uuid.New(), OrgID: orgID, Name: "seed-team-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create seed team: %v", err)
	}
	return team.ID
}
