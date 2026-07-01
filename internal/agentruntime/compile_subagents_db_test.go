package agentruntime

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

// A user-created agent (no catalog link) owns its sub-agents as rows in the
// agents table; the compiler should load and compile them.
func TestCompile_LoadsUserAgentSubAgentsFromDB(t *testing.T) {
	db := connectCompileTestDB(t)
	org := createOrg(t, db)
	parent := createUserAgent(t, db, org.ID, "Aria")

	reviewer := createSubAgent(t, db, org.ID, parent.ID, subAgentSeed{
		Name:         "Reviewer",
		Description:  "Reviews delegated work.",
		Instructions: "Review carefully before returning.",
		Tools:        model.JSON{"read_file": true},
	})
	researcher := createSubAgent(t, db, org.ID, parent.ID, subAgentSeed{
		Name:         "Researcher",
		Description:  "Researches delegated work.",
		Instructions: "Gather context first.",
		Tools:        model.JSON{"grep": true},
	})

	def, err := Compile(context.Background(), CompileDeps{DB: db, Cfg: &config.Config{}}, &parent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	if len(def.SubAgents) != 2 {
		t.Fatalf("subagents = %#v, want 2", def.SubAgents)
	}
	got := def.SubAgents[reviewer.ID.String()]
	if got == nil {
		t.Fatalf("missing reviewer subagent: %#v", def.SubAgents)
	}
	if got.Agent.Name != "Reviewer" || got.Agent.Description != "Reviews delegated work." {
		t.Fatalf("reviewer meta = %#v", got.Agent)
	}
	if !containsString(runtimeToolTypes(got.Tools), "builtin.read_file") {
		t.Fatalf("reviewer tools = %#v, want read_file", runtimeToolTypes(got.Tools))
	}
	if def.SubAgents[researcher.ID.String()] == nil {
		t.Fatalf("missing researcher subagent: %#v", def.SubAgents)
	}
}

// Archived / mislabeled rows must not be picked up as sub-agents.
func TestCompile_IgnoresArchivedUserSubAgents(t *testing.T) {
	db := connectCompileTestDB(t)
	org := createOrg(t, db)
	parent := createUserAgent(t, db, org.ID, "Aria")

	active := createSubAgent(t, db, org.ID, parent.ID, subAgentSeed{Name: "Active", Tools: model.JSON{"read_file": true}})
	archived := createSubAgent(t, db, org.ID, parent.ID, subAgentSeed{Name: "Archived", Tools: model.JSON{"read_file": true}})
	if err := db.Model(&model.Agent{}).Where("id = ?", archived.ID).Update("status", "archived").Error; err != nil {
		t.Fatalf("archive subagent: %v", err)
	}

	def, err := Compile(context.Background(), CompileDeps{DB: db, Cfg: &config.Config{}}, &parent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(def.SubAgents) != 1 || def.SubAgents[active.ID.String()] == nil {
		t.Fatalf("subagents = %#v, want only the active one", def.SubAgents)
	}
}

// Two different parents may each own a sub-agent named "Worker" (uniqueness is
// per-parent), while top-level agent names remain unique per org.
func TestSubAgentNameUniquenessIsPerParent(t *testing.T) {
	db := connectCompileTestDB(t)
	org := createOrg(t, db)
	parentA := createUserAgent(t, db, org.ID, "Parent A")
	parentB := createUserAgent(t, db, org.ID, "Parent B")

	createSubAgent(t, db, org.ID, parentA.ID, subAgentSeed{Name: "Worker"})
	createSubAgent(t, db, org.ID, parentB.ID, subAgentSeed{Name: "Worker"})

	// Same name under the same parent is rejected.
	dup := userAgentRow(org.ID, "Worker")
	dup.Type = model.AgentTypeSubAgent
	dup.ParentAgentID = &parentA.ID
	if err := db.Create(&dup).Error; err == nil {
		t.Fatalf("expected duplicate sub-agent name under the same parent to be rejected")
	}

	// A top-level agent name collision within the org is still rejected.
	dupTop := userAgentRow(org.ID, "Parent A")
	if err := db.Create(&dupTop).Error; err == nil {
		t.Fatalf("expected duplicate top-level agent name to be rejected")
	}
}

type subAgentSeed struct {
	Name         string
	Description  string
	Instructions string
	Tools        model.JSON
}

func createOrg(t *testing.T, db *gorm.DB) model.Org {
	t.Helper()
	org := model.Org{Name: "Subagent db-" + uuid.NewString()}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	return org
}

func userAgentRow(orgID uuid.UUID, name string) model.Agent {
	return model.Agent{
		ID:            uuid.New(),
		OrgID:         &orgID,
		Type:          model.AgentTypeAgent,
		Name:          name,
		Model:         DefaultAgentModel,
		Tools:         model.JSON{},
		McpServers:    model.RawJSON("[]"),
		Skills:        model.JSON{},
		Integrations:  model.JSON{},
		Resources:     model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
	}
}

func createUserAgent(t *testing.T, db *gorm.DB, orgID uuid.UUID, name string) model.Agent {
	t.Helper()
	agent := userAgentRow(orgID, name)
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create user agent: %v", err)
	}
	return agent
}

func createSubAgent(t *testing.T, db *gorm.DB, orgID, parentID uuid.UUID, seed subAgentSeed) model.Agent {
	t.Helper()
	tools := seed.Tools
	if tools == nil {
		tools = model.JSON{}
	}
	desc := seed.Description
	instructions := seed.Instructions
	agent := userAgentRow(orgID, seed.Name)
	agent.Type = model.AgentTypeSubAgent
	agent.ParentAgentID = &parentID
	agent.Description = &desc
	agent.Instructions = &instructions
	agent.Tools = tools
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create sub-agent %s: %v", seed.Name, err)
	}
	return agent
}
