package skills

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestSkillManagementMCPPersistsBundleAndScopesItToCallingAgentTeam(t *testing.T) {
	db := connectSkillManagementTestDB(t)
	org := model.Org{ID: uuid.New(), Name: "skill-mcp-" + uuid.NewString()[:8], Active: true, RateLimit: 1000}
	teamA := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "skill-team-a-" + uuid.NewString()[:8]}
	teamB := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "skill-team-b-" + uuid.NewString()[:8]}
	memberA := model.User{ID: uuid.New(), Email: "skill-a-" + uuid.NewString()[:8] + "@example.com", Name: "Member A"}
	memberB := model.User{ID: uuid.New(), Email: "skill-b-" + uuid.NewString()[:8] + "@example.com", Name: "Member B"}
	agentA := model.Agent{
		ID: uuid.New(), OrgID: &org.ID, TeamID: teamA.ID, Name: "Agent A", Model: "test",
		Status: "active",
	}
	agentB := model.Agent{
		ID: uuid.New(), OrgID: &org.ID, TeamID: teamB.ID, Name: "Agent B", Model: "test",
		Status: "active",
	}
	rows := []any{
		&org,
		&memberA,
		&memberB,
		&model.OrgMembership{UserID: memberA.ID, OrgID: org.ID, Role: "member"},
		&model.OrgMembership{UserID: memberB.ID, OrgID: org.ID, Role: "member"},
		&teamA,
		&teamB,
		&model.TeamMember{OrgID: org.ID, TeamID: teamA.ID, UserID: memberA.ID, Role: "member"},
		&model.TeamMember{OrgID: org.ID, TeamID: teamB.ID, UserID: memberB.ID, Role: "member"},
		&agentA,
		&agentB,
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Skill{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		db.Where("id IN ?", []uuid.UUID{memberA.ID, memberB.ID}).Delete(&model.User{})
		db.Delete(&model.Org{}, "id = ?", org.ID)
	})

	client := connectedSkillManagementClient(t, db, org.ID, agentA.ID)
	tools, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("list skill tools: %v", err)
	}
	var createTool *mcp.Tool
	for _, tool := range tools.Tools {
		if tool.Name == toolCreateSkill {
			createTool = tool
			break
		}
	}
	if createTool == nil {
		t.Fatal("create_skill was not registered")
	}
	properties := createTool.InputSchema.(map[string]any)["properties"].(map[string]any)
	if _, exposed := properties["team_id"]; exposed {
		t.Fatalf("create_skill must never accept team_id: %#v", properties)
	}

	largeReference := strings.Repeat("production reference\n", 20_000)
	createResult, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: toolCreateSkill,
		Arguments: map[string]any{
			"entrypoint_content": `---
name: platform-status
description: Use when checking the production platform status.
human_description: Check production health using the standard runbook.
category: platform
tags: [status, production]
required_environment_variables: [STATUS_API_TOKEN]
---

# Platform status

Read references/runbook.md, then execute scripts/check.sh.
`,
			"files": map[string]string{
				"references/runbook.md": "# Runbook\nCheck every regional endpoint.\n",
				"references/large.md":   largeReference,
				"scripts/check.sh":      "#!/bin/sh\ncurl -fsS \"$STATUS_URL\"\n",
			},
			"_hivy_actor_user_id": memberA.ID.String(),
		},
	})
	if err != nil {
		t.Fatalf("call create_skill: %v", err)
	}
	if createResult.IsError {
		t.Fatalf("create_skill returned error: %s", skillManagementResultText(createResult))
	}

	var created model.Skill
	if err := db.Where("org_id = ? AND slug = ?", org.ID, "platform-status").First(&created).Error; err != nil {
		t.Fatalf("load created skill: %v", err)
	}
	if created.TeamID == nil || *created.TeamID != teamA.ID {
		t.Fatalf("created skill team_id = %v, want calling agent team %s", created.TeamID, teamA.ID)
	}
	if created.OrgID == nil || *created.OrgID != org.ID {
		t.Fatalf("created skill org_id = %v, want %s", created.OrgID, org.ID)
	}
	if created.PublisherID == nil || *created.PublisherID != memberA.ID {
		t.Fatalf("created skill publisher_id = %v, want actor %s", created.PublisherID, memberA.ID)
	}
	if created.Status != model.SkillStatusPublished || created.Category != "platform" {
		t.Fatalf("created skill status/category = %q/%q", created.Status, created.Category)
	}
	if created.Description == nil || *created.Description != "Use when checking the production platform status." {
		t.Fatalf("created description = %v", created.Description)
	}
	if created.HumanDescription == nil || *created.HumanDescription != "Check production health using the standard runbook." {
		t.Fatalf("created human description = %v", created.HumanDescription)
	}
	if strings.Join(created.Tags, ",") != "status,production" {
		t.Fatalf("created tags = %v", created.Tags)
	}
	bundle, err := decodeSkillBundle(created)
	if err != nil {
		t.Fatalf("decode created bundle: %v", err)
	}
	if bundle.Content != "# Platform status\n\nRead references/runbook.md, then execute scripts/check.sh.\n" {
		t.Fatalf("created entrypoint body = %q", bundle.Content)
	}
	if bundle.Files["references/large.md"] != largeReference {
		t.Fatalf("large supporting file was not persisted through raised limits")
	}
	if bundle.Files["scripts/check.sh"] == "" || len(bundle.Files) != 3 {
		t.Fatalf("created supporting files = %#v", bundle.Files)
	}
	if strings.Join(bundle.RequiredEnvironmentVariables, ",") != "STATUS_API_TOKEN" {
		t.Fatalf("required environment variables = %v", bundle.RequiredEnvironmentVariables)
	}

	availableToA, err := loadAgentPublishedSkills(t.Context(), db, agentA.ID)
	if err != nil {
		t.Fatalf("resolve team A skills: %v", err)
	}
	availableToB, err := loadAgentPublishedSkills(t.Context(), db, agentB.ID)
	if err != nil {
		t.Fatalf("resolve team B skills: %v", err)
	}
	if len(availableToA) != 1 || availableToA[0].ID != created.ID {
		t.Fatalf("team A skills = %#v", availableToA)
	}
	if len(availableToB) != 0 {
		t.Fatalf("team B must not inherit team A skill: %#v", availableToB)
	}

	if err := db.Model(&created).Update("status", model.SkillStatusArchived).Error; err != nil {
		t.Fatalf("archive before update: %v", err)
	}
	updateResult, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: toolUpdateSkill,
		Arguments: map[string]any{
			"skill": "platform-status",
			"entrypoint_content": `---
name: platform-status
description: Use when investigating a production platform incident.
category: reliability
tags: [incident]
---

# Incident investigation

Execute scripts/investigate.sh.
`,
			"files": map[string]string{
				"scripts/investigate.sh": "#!/bin/sh\nset -eu\n",
			},
			"_hivy_actor_user_id": memberA.ID.String(),
		},
	})
	if err != nil {
		t.Fatalf("call update_skill: %v", err)
	}
	if updateResult.IsError {
		t.Fatalf("update_skill returned error: %s", skillManagementResultText(updateResult))
	}

	if err := db.First(&created, "id = ?", created.ID).Error; err != nil {
		t.Fatalf("reload updated skill: %v", err)
	}
	if created.TeamID == nil || *created.TeamID != teamA.ID {
		t.Fatalf("updated skill escaped calling agent team: %v", created.TeamID)
	}
	if created.Status != model.SkillStatusPublished || created.Category != "reliability" {
		t.Fatalf("updated status/category = %q/%q", created.Status, created.Category)
	}
	if created.HumanDescription != nil {
		t.Fatalf("omitted human_description must be cleared, got %q", *created.HumanDescription)
	}
	updatedBundle, err := decodeSkillBundle(created)
	if err != nil {
		t.Fatalf("decode updated bundle: %v", err)
	}
	updatedFiles := skillBundleFiles(updatedBundle)
	if len(updatedFiles) != 1 || updatedFiles["scripts/investigate.sh"] == "" {
		t.Fatalf("update must replace the entire supporting file set: %#v", updatedFiles)
	}
	if _, retained := updatedFiles["references/runbook.md"]; retained {
		t.Fatal("update retained an omitted supporting file")
	}

	deniedResult, err := client.CallTool(t.Context(), &mcp.CallToolParams{
		Name: toolCreateSkill,
		Arguments: map[string]any{
			"entrypoint_content":  "---\nname: forbidden-skill\ndescription: Use when this must not be created.\n---\n\n# Forbidden\n",
			"_hivy_actor_user_id": memberB.ID.String(),
		},
	})
	if err != nil {
		t.Fatalf("call denied create_skill: %v", err)
	}
	if !deniedResult.IsError || !strings.Contains(skillManagementResultText(deniedResult), "requires membership") {
		t.Fatalf("cross-team actor must be rejected, got: %s", skillManagementResultText(deniedResult))
	}
	var forbiddenCount int64
	if err := db.Model(&model.Skill{}).
		Where("org_id = ? AND slug = ?", org.ID, "forbidden-skill").
		Count(&forbiddenCount).Error; err != nil {
		t.Fatalf("count forbidden skill: %v", err)
	}
	if forbiddenCount != 0 {
		t.Fatalf("cross-team actor created %d forbidden skills", forbiddenCount)
	}
}

func connectSkillManagementTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(testdb.DatabaseURL()), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func connectedSkillManagementClient(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "skill-management-test", Version: "1"}, nil)
	NewToolsFunc(db, "https://app.example.test")(server, &model.Token{
		OrgID: orgID,
		Meta: model.JSON{
			model.TokenMetaType:    model.TokenTypeAgentProxy,
			model.TokenMetaAgentID: agentID.String(),
		},
	})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("connect skill MCP server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "skill-management-client", Version: "1"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect skill MCP client: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return clientSession
}

func skillManagementResultText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			return text.Text
		}
	}
	return ""
}
