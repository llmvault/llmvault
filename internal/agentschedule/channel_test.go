package agentschedule

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

type scheduleChannelFixture struct {
	org      model.Org
	team     model.Team
	agent    model.Agent
	session  model.Session
	channel  model.Channel
	target   model.Channel
	teamHome model.Channel
}

func connectScheduleTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(testdb.DatabaseURL()), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func seedScheduleChannelFixture(t *testing.T, db *gorm.DB) scheduleChannelFixture {
	t.Helper()
	org := model.Org{ID: uuid.New(), Name: "schedule-channel-" + uuid.NewString(), Active: true, RateLimit: 1000}
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-" + uuid.NewString()}
	agent := model.Agent{ID: uuid.New(), OrgID: &org.ID, TeamID: &team.ID, Name: "Schedule Agent " + uuid.NewString(), Model: "test", Status: "active"}
	channel := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "work-" + uuid.NewString(), DefaultAgentID: agent.ID}
	target := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "ops-" + uuid.NewString(), DefaultAgentID: agent.ID}
	teamHome := model.Channel{ID: uuid.New(), OrgID: org.ID, TeamID: &team.ID, Name: "general", Kind: "standard", Visibility: "public", DefaultAgentID: agent.ID, IsDefault: true}
	session := model.Session{ID: uuid.New(), OrgID: org.ID, ChannelID: channel.ID, AgentID: agent.ID, Status: "active"}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("create target channel: %v", err)
	}
	if err := db.Create(&teamHome).Error; err != nil {
		t.Fatalf("create team #general channel: %v", err)
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() { cleanupScheduleChannelOrg(t, db, org.ID) })
	return scheduleChannelFixture{org: org, team: team, agent: agent, session: session, channel: channel, target: target, teamHome: teamHome}
}

func cleanupScheduleChannelOrg(t *testing.T, db *gorm.DB, orgID uuid.UUID) {
	t.Helper()
	db.Where("org_id = ?", orgID).Delete(&model.AgentScheduleRun{})
	db.Where("org_id = ?", orgID).Delete(&model.AgentSchedule{})
	db.Where("org_id = ?", orgID).Delete(&model.Session{})
	db.Where("org_id = ?", orgID).Delete(&model.Channel{})
	db.Where("org_id = ?", orgID).Delete(&model.Agent{})
	db.Where("org_id = ?", orgID).Delete(&model.Team{})
	db.Delete(&model.Org{}, "id = ?", orgID)
}

func TestCreateFromSessionDefaultsToTeamGeneral(t *testing.T) {
	db := connectScheduleTestDB(t)
	fx := seedScheduleChannelFixture(t, db)
	interval := int64(60)
	schedule, err := CreateFromSession(t.Context(), db, &fx.agent, fx.session.ID.String(), CreateInput{
		JobID:           "job-" + uuid.NewString(),
		TaskPrompt:      "summarize weekly activity",
		IntervalSeconds: &interval,
	})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	if schedule.Channel != fx.teamHome.ID.String() {
		t.Fatalf("schedule channel = %s, want team #general %s", schedule.Channel, fx.teamHome.ID)
	}
	// No system channel is ever created: the only channels remain the seeded ones.
	var systemCount int64
	if err := db.Model(&model.Channel{}).Where("org_id = ? AND kind = ?", fx.org.ID, "system").Count(&systemCount).Error; err != nil {
		t.Fatalf("count system channels: %v", err)
	}
	if systemCount != 0 {
		t.Fatalf("created %d system channels", systemCount)
	}
}

func TestCreateFromSessionTeamlessAgentRequiresExplicitChannel(t *testing.T) {
	db := connectScheduleTestDB(t)
	fx := seedScheduleChannelFixture(t, db)
	// Strip the agent's team: a legacy org-level agent has no team #general.
	if err := db.Model(&model.Agent{}).Where("id = ?", fx.agent.ID).Update("team_id", nil).Error; err != nil {
		t.Fatalf("clear agent team: %v", err)
	}
	fx.agent.TeamID = nil
	interval := int64(60)
	_, err := CreateFromSession(t.Context(), db, &fx.agent, fx.session.ID.String(), CreateInput{
		JobID:           "job-" + uuid.NewString(),
		TaskPrompt:      "summarize weekly activity",
		IntervalSeconds: &interval,
	})
	if err == nil || !strings.Contains(err.Error(), "agent has no team") {
		t.Fatalf("teamless default channel error = %v, want agent has no team", err)
	}
}

func TestCreateFromSessionUsesSelectedChannel(t *testing.T) {
	db := connectScheduleTestDB(t)
	fx := seedScheduleChannelFixture(t, db)
	interval := int64(60)
	schedule, err := CreateFromSession(t.Context(), db, &fx.agent, fx.session.ID.String(), CreateInput{
		JobID:           "job-" + uuid.NewString(),
		TaskPrompt:      "summarize ops",
		ChannelID:       fx.target.ID.String(),
		IntervalSeconds: &interval,
	})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	if schedule.Channel != fx.target.ID.String() {
		t.Fatalf("schedule channel = %s, want %s", schedule.Channel, fx.target.ID)
	}
	var count int64
	if err := db.Model(&model.Channel{}).Where("org_id = ? AND kind = ?", fx.org.ID, "system").Count(&count).Error; err != nil {
		t.Fatalf("count system channels: %v", err)
	}
	if count != 0 {
		t.Fatalf("created %d system channels for explicit channel", count)
	}
}

func TestScheduleChannelMustBelongToOrgAndBeActive(t *testing.T) {
	db := connectScheduleTestDB(t)
	fx := seedScheduleChannelFixture(t, db)
	other := seedScheduleChannelFixture(t, db)
	archivedAt := time.Now().UTC()
	archived := model.Channel{
		ID:             uuid.New(),
		OrgID:          fx.org.ID,
		Name:           "archived-" + uuid.NewString(),
		DefaultAgentID: fx.agent.ID,
		ArchivedAt:     &archivedAt,
	}
	if err := db.Create(&archived).Error; err != nil {
		t.Fatalf("create archived channel: %v", err)
	}
	interval := int64(60)
	for _, channelID := range []string{other.target.ID.String(), archived.ID.String()} {
		_, err := CreateFromSession(t.Context(), db, &fx.agent, fx.session.ID.String(), CreateInput{
			JobID:           "job-" + uuid.NewString(),
			TaskPrompt:      "summarize",
			ChannelID:       channelID,
			IntervalSeconds: &interval,
		})
		if err == nil || !strings.Contains(err.Error(), "channel_id not found") {
			t.Fatalf("create with channel %s error = %v, want channel_id not found", channelID, err)
		}
	}
}

func TestScheduleChannelRejectsProviderChannelID(t *testing.T) {
	db := connectScheduleTestDB(t)
	fx := seedScheduleChannelFixture(t, db)
	interval := int64(60)
	_, err := CreateFromSession(t.Context(), db, &fx.agent, fx.session.ID.String(), CreateInput{
		JobID:           "job-" + uuid.NewString(),
		TaskPrompt:      "summarize",
		ChannelID:       "C0123ABCD",
		IntervalSeconds: &interval,
	})
	if err == nil || !strings.Contains(err.Error(), "channel_id must be a uuid") {
		t.Fatalf("create with slack channel id error = %v, want channel_id must be a uuid", err)
	}
}

func TestUpdateCanSelectScheduleChannel(t *testing.T) {
	db := connectScheduleTestDB(t)
	fx := seedScheduleChannelFixture(t, db)
	interval := int64(60)
	jobID := "job-" + uuid.NewString()
	if _, err := CreateFromSession(t.Context(), db, &fx.agent, fx.session.ID.String(), CreateInput{
		JobID:           jobID,
		TaskPrompt:      "summarize weekly activity",
		IntervalSeconds: &interval,
	}); err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	channelID := fx.target.ID.String()
	updated, err := Update(t.Context(), db, &fx.agent, jobID, UpdateInput{ChannelID: &channelID})
	if err != nil {
		t.Fatalf("update schedule: %v", err)
	}
	if updated.Channel != channelID {
		t.Fatalf("updated channel = %s, want %s", updated.Channel, channelID)
	}
}

// TestSelectedScheduleChannelRejectsForeignTeamChannel verifies the team-primary
// rule (channelagents.ActsInChannel) that replaced the cut agent_channels
// allowlist: an agent may not be scheduled into a channel owned by a team it does
// not belong to, while its own team's channels remain schedulable.
func TestSelectedScheduleChannelRejectsForeignTeamChannel(t *testing.T) {
	db := connectScheduleTestDB(t)
	fx := seedScheduleChannelFixture(t, db)
	foreignTeam := model.Team{ID: uuid.New(), OrgID: fx.org.ID, Name: "foreign-" + uuid.NewString()}
	if err := db.Create(&foreignTeam).Error; err != nil {
		t.Fatalf("create foreign team: %v", err)
	}
	foreignChannel := model.Channel{ID: uuid.New(), OrgID: fx.org.ID, TeamID: &foreignTeam.ID, Name: "foreign-ops-" + uuid.NewString(), DefaultAgentID: fx.agent.ID}
	if err := db.Create(&foreignChannel).Error; err != nil {
		t.Fatalf("create foreign channel: %v", err)
	}
	interval := int64(60)
	_, err := CreateFromSession(t.Context(), db, &fx.agent, fx.session.ID.String(), CreateInput{
		JobID:           "job-" + uuid.NewString(),
		TaskPrompt:      "summarize foreign channel",
		ChannelID:       foreignChannel.ID.String(),
		IntervalSeconds: &interval,
	})
	if err == nil || !strings.Contains(err.Error(), "agent is not available in this channel") {
		t.Fatalf("create in foreign-team channel error = %v, want agent restriction", err)
	}
	allowed, err := CreateFromSession(t.Context(), db, &fx.agent, fx.session.ID.String(), CreateInput{
		JobID:           "job-" + uuid.NewString(),
		TaskPrompt:      "summarize allowed channel",
		ChannelID:       fx.teamHome.ID.String(),
		IntervalSeconds: &interval,
	})
	if err != nil {
		t.Fatalf("create in allowed channel: %v", err)
	}
	if allowed.Channel != fx.teamHome.ID.String() {
		t.Fatalf("allowed channel = %s, want %s", allowed.Channel, fx.teamHome.ID)
	}
}
