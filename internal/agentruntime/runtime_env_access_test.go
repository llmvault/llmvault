package agentruntime

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

func TestBuildRuntimeEnvExcludesAgentDisabledTeamVariables(t *testing.T) {
	db := connectCompileTestDB(t)
	ctx := context.Background()
	org := model.Org{ID: uuid.New(), Name: "runtime-env-" + uuid.NewString()[:8], Active: true, RateLimit: 1000}
	if err := db.WithContext(ctx).Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	team := seedCompileTeam(t, db, org.ID)
	agent := model.Agent{
		ID: uuid.New(), OrgID: &org.ID, TeamID: team.ID, Name: "Environment agent",
		Model: DefaultAgentModel, Status: "active", Tools: model.JSON{}, McpServers: model.RawJSON("[]"),
		Skills: model.JSON{}, RuntimeConfig: model.JSON{}, Permissions: model.JSON{}, Resources: model.JSON{},
	}
	if err := db.WithContext(ctx).Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	key, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32)))
	if err != nil {
		t.Fatalf("create encryption key: %v", err)
	}
	visibleValue, err := key.EncryptString("visible-secret")
	if err != nil {
		t.Fatalf("encrypt visible value: %v", err)
	}
	hiddenValue, err := key.EncryptString("hidden-secret")
	if err != nil {
		t.Fatalf("encrypt hidden value: %v", err)
	}
	visible := model.TeamEnvVar{
		ID: uuid.New(), OrgID: org.ID, TeamID: team.ID, Name: "VISIBLE_TOKEN",
		EncryptedValue: visibleValue, Description: "Visible runtime credential",
	}
	hidden := model.TeamEnvVar{
		ID: uuid.New(), OrgID: org.ID, TeamID: team.ID, Name: "HIDDEN_TOKEN",
		EncryptedValue: hiddenValue, Description: "Hidden runtime credential",
	}
	if err := db.WithContext(ctx).Create(&[]model.TeamEnvVar{visible, hidden}).Error; err != nil {
		t.Fatalf("create team environment variables: %v", err)
	}
	if err := db.WithContext(ctx).Create(&model.AgentTeamEnvVarDeny{
		OrgID: org.ID, AgentID: agent.ID, TeamEnvVarID: hidden.ID,
	}).Error; err != nil {
		t.Fatalf("disable team environment variable: %v", err)
	}

	deps := CompileDeps{DB: db, EncKey: key}
	env, err := BuildRuntimeEnvWithProxyToken(
		ctx,
		deps,
		&agent,
		nil,
		"runtime-secret",
		&ProxyTokenResult{Token: "proxy-token", JTI: "proxy-jti"},
		team.ID,
	)
	if err != nil {
		t.Fatalf("build runtime environment: %v", err)
	}
	if got := env[teamEnvInjectPrefix+"VISIBLE_TOKEN"]; got != "visible-secret" {
		t.Fatalf("visible team environment value = %q", got)
	}
	if _, ok := env[teamEnvInjectPrefix+"HIDDEN_TOKEN"]; ok {
		t.Fatal("disabled team environment variable was injected")
	}

	def := &AgentDefinition{}
	if err := appendTeamEnvVarPromptDoc(ctx, deps, def, org.ID, team.ID, agent.ID); err != nil {
		t.Fatalf("append team environment prompt: %v", err)
	}
	dynamic := requireDynamicSegments(t, def.SystemPrompt)
	segment := requireStaticPromptSegmentByTitle(t, dynamic, "Team environment variables")
	content := requirePromptString(t, segment.Content)
	if !strings.Contains(content, "VISIBLE_TOKEN") {
		t.Fatalf("prompt does not describe enabled variable: %q", content)
	}
	if strings.Contains(content, "HIDDEN_TOKEN") {
		t.Fatalf("prompt describes disabled variable: %q", content)
	}
}
