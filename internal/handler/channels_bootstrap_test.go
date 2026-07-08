package handler

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestCreateUserDefaultOrg_ProvisionsFirstTeamHivyAndGeneral(t *testing.T) {
	db := connectInternalTestDB(t)
	user := seedSignupUser(t, db)

	var org model.Org
	err := db.Transaction(func(tx *gorm.DB) error {
		var e error
		org, e = createUserDefaultOrg(context.Background(), tx, nil, user, "Platform")
		return e
	})
	if err != nil {
		t.Fatalf("createUserDefaultOrg: %v", err)
	}
	cleanupOrgAndLedger(t, db, org.ID)

	// The forced first team exists with the provided name and the owner as member.
	var team model.Team
	if err := db.Where("org_id = ? AND name = ?", org.ID, "Platform").First(&team).Error; err != nil {
		t.Fatalf("load first team: %v", err)
	}
	var teamMemberCount int64
	if err := db.Model(&model.TeamMember{}).
		Where("team_id = ? AND user_id = ?", team.ID, user.ID).Count(&teamMemberCount).Error; err != nil {
		t.Fatalf("count team members: %v", err)
	}
	if teamMemberCount != 1 {
		t.Fatalf("owner team membership count = %d, want 1", teamMemberCount)
	}

	// The team's default Hivy: IsDefault, team-scoped, undeletable.
	var hivy model.Agent
	if err := db.Where("org_id = ? AND team_id = ? AND is_default = true", org.ID, team.ID).First(&hivy).Error; err != nil {
		t.Fatalf("load team Hivy: %v", err)
	}

	// The team's #general: team-scoped, default agent = the team Hivy, owner member.
	var general model.Channel
	if err := db.Where("org_id = ? AND team_id = ? AND name = ?", org.ID, team.ID, defaultChannelName).First(&general).Error; err != nil {
		t.Fatalf("load #general channel: %v", err)
	}
	if !general.IsDefault || general.Kind != "standard" || general.DefaultAgentID != hivy.ID {
		t.Fatalf("#general = %#v (want default agent %s)", general, hivy.ID)
	}
	var chMemberCount int64
	if err := db.Model(&model.ChannelMember{}).
		Where("channel_id = ? AND user_id = ?", general.ID, user.ID).Count(&chMemberCount).Error; err != nil {
		t.Fatalf("count channel members: %v", err)
	}
	if chMemberCount != 1 {
		t.Fatalf("owner #general membership count = %d, want 1", chMemberCount)
	}

	// No #system channel is created anywhere.
	var systemCount int64
	if err := db.Model(&model.Channel{}).Where("org_id = ? AND kind = ?", org.ID, "system").Count(&systemCount).Error; err != nil {
		t.Fatalf("count system channels: %v", err)
	}
	if systemCount != 0 {
		t.Fatalf("system channel count = %d, want 0", systemCount)
	}
}

func TestCreateUserDefaultOrg_RequiresTeamName(t *testing.T) {
	db := connectInternalTestDB(t)
	user := seedSignupUser(t, db)

	err := db.Transaction(func(tx *gorm.DB) error {
		_, e := createUserDefaultOrg(context.Background(), tx, nil, user, "   ")
		return e
	})
	if err == nil {
		t.Fatalf("expected error for empty team name")
	}
}
