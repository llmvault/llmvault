package mcpserver

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// seedCronSchedule inserts an active interval schedule for the fixture's agent
// bound to the given channel id, returning its runtime job id.
func seedCronSchedule(t *testing.T, db *gorm.DB, fx actorEnforcementFixture, channelID uuid.UUID) string {
	t.Helper()
	jobID := "cron-" + uuid.NewString()
	interval := int64(3600)
	row := model.AgentSchedule{
		OrgID:           fx.org.ID,
		AgentID:         fx.agent.ID,
		RuntimeJobID:    jobID,
		Status:          "active",
		ScheduleKind:    "interval",
		Channel:         channelID.String(),
		TaskPrompt:      "do a thing",
		Description:     "desc",
		IntervalSeconds: &interval,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed schedule: %v", err)
	}
	t.Cleanup(func() {
		db.Where("agent_id = ? AND runtime_job_id = ?", fx.agent.ID, jobID).Delete(&model.AgentSchedule{})
	})
	return jobID
}

// cronJobIDs decodes the job_id values from a cron "list" result.
func cronJobIDs(t *testing.T, result *mcp.CallToolResult) map[string]bool {
	t.Helper()
	out := decodeChannelToolResult(t, result)
	jobs, _ := out["jobs"].([]any)
	ids := map[string]bool{}
	for _, item := range jobs {
		entry, _ := item.(map[string]any)
		if id, ok := entry["job_id"].(string); ok {
			ids[id] = true
		}
	}
	return ids
}

func TestCronListHidesSchedulesActorCannotUse(t *testing.T) {
	db := connectChannelToolTestDB(t)
	fx := seedActorEnforcementFixture(t, db)

	usableJob := seedCronSchedule(t, db, fx, fx.general.ID)
	hiddenJob := seedCronSchedule(t, db, fx, fx.teamChannel.ID)

	// Member on neither team: sees only the usable-channel schedule.
	memberResult, err := handleCronTool(t.Context(), db, &fx.agent, cronToolArgs{
		Action: "list", HivyActorUserID: fx.member.ID.String(),
	})
	if err != nil {
		t.Fatalf("handleCronTool list member: %v", err)
	}
	ids := cronJobIDs(t, memberResult)
	if !ids[usableJob] {
		t.Fatalf("member should see usable-channel job; got %v", ids)
	}
	if ids[hiddenJob] {
		t.Fatalf("member must not see other-team job; got %v", ids)
	}

	// Admin sees both.
	adminResult, err := handleCronTool(t.Context(), db, &fx.agent, cronToolArgs{
		Action: "list", HivyActorUserID: fx.admin.ID.String(),
	})
	if err != nil {
		t.Fatalf("handleCronTool list admin: %v", err)
	}
	adminIDs := cronJobIDs(t, adminResult)
	if !adminIDs[usableJob] || !adminIDs[hiddenJob] {
		t.Fatalf("admin should see all jobs; got %v", adminIDs)
	}

	// Automated run (no actor) keeps agent-scoped visibility of all jobs.
	autoResult, err := handleCronTool(t.Context(), db, &fx.agent, cronToolArgs{Action: "list"})
	if err != nil {
		t.Fatalf("handleCronTool list no-actor: %v", err)
	}
	autoIDs := cronJobIDs(t, autoResult)
	if !autoIDs[usableJob] || !autoIDs[hiddenJob] {
		t.Fatalf("automated run should see all jobs; got %v", autoIDs)
	}
}

func TestCronManageDeniedForSchedulesActorCannotUse(t *testing.T) {
	db := connectChannelToolTestDB(t)
	fx := seedActorEnforcementFixture(t, db)

	hiddenJob := seedCronSchedule(t, db, fx, fx.teamChannel.ID)
	usableJob := seedCronSchedule(t, db, fx, fx.general.ID)

	// Member cannot pause a schedule in a channel they aren't in.
	pauseResult, err := handleCronTool(t.Context(), db, &fx.agent, cronToolArgs{
		Action: "pause", JobID: hiddenJob, HivyActorUserID: fx.member.ID.String(),
	})
	if err != nil {
		t.Fatalf("handleCronTool pause: %v", err)
	}
	if msg := decodeToolError(t, pauseResult); !strings.Contains(msg, "channels you belong to") {
		t.Fatalf("expected channel-membership denial, got %q", msg)
	}

	// Member cannot cancel it either.
	cancelResult, err := handleCronTool(t.Context(), db, &fx.agent, cronToolArgs{
		Action: "cancel", JobID: hiddenJob, HivyActorUserID: fx.member.ID.String(),
	})
	if err != nil {
		t.Fatalf("handleCronTool cancel: %v", err)
	}
	if msg := decodeToolError(t, cancelResult); !strings.Contains(msg, "channels you belong to") {
		t.Fatalf("expected channel-membership denial on cancel, got %q", msg)
	}

	// Member CAN pause a schedule in a channel they can use.
	okResult, err := handleCronTool(t.Context(), db, &fx.agent, cronToolArgs{
		Action: "pause", JobID: usableJob, HivyActorUserID: fx.member.ID.String(),
	})
	if err != nil {
		t.Fatalf("handleCronTool pause usable: %v", err)
	}
	if okResult.IsError {
		t.Fatalf("member should be able to pause a usable-channel job: %v", okResult.Content)
	}

	// Admin can manage the restricted-channel schedule.
	adminResult, err := handleCronTool(t.Context(), db, &fx.agent, cronToolArgs{
		Action: "pause", JobID: hiddenJob, HivyActorUserID: fx.admin.ID.String(),
	})
	if err != nil {
		t.Fatalf("handleCronTool pause admin: %v", err)
	}
	if adminResult.IsError {
		t.Fatalf("admin should be able to pause any job: %v", adminResult.Content)
	}

	// Automated run (no actor) is unrestricted too.
	autoResult, err := handleCronTool(t.Context(), db, &fx.agent, cronToolArgs{
		Action: "resume", JobID: hiddenJob,
	})
	if err != nil {
		t.Fatalf("handleCronTool resume no-actor: %v", err)
	}
	if autoResult.IsError {
		t.Fatalf("automated run should be unrestricted: %v", autoResult.Content)
	}
}
