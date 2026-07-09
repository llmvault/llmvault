package agents

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

// TestRequireOrgManager verifies the agent-builder tools are gated on the acting
// human's org role, with a clear, relayable error for non-managers and a
// preserved (non-blocking) path for automated runs with no actor.
func TestRequireOrgManager(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)

	member := model.User{ID: uuid.New(), Email: "member-" + uuid.NewString() + "@example.test", Name: "Member"}
	admin := model.User{ID: uuid.New(), Email: "admin-" + uuid.NewString() + "@example.test", Name: "Admin"}
	for _, row := range []any{
		&member, &admin,
		&model.OrgMembership{UserID: member.ID, OrgID: org.ID, Role: "member"},
		&model.OrgMembership{UserID: admin.ID, OrgID: org.ID, Role: "owner"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed row %T: %v", row, err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		db.Delete(&model.User{}, "id IN ?", []uuid.UUID{member.ID, admin.ID})
	})

	reqFor := func(actorID string) *mcp.CallToolRequest {
		args, _ := json.Marshal(map[string]any{"_hivy_actor_user_id": actorID})
		return &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: args}}
	}

	// A plain member is blocked with a clear, role-specific message.
	blocked := requireOrgManager(t.Context(), db, org.ID, reqFor(member.ID.String()), "creating an agent")
	if blocked == nil {
		t.Fatal("member should be blocked from creating agents")
	}
	msg := blocked.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(msg, "admin or owner") || !strings.Contains(msg, "member") {
		t.Fatalf("error should name the required and actual role clearly: %q", msg)
	}

	// An org owner is allowed.
	if res := requireOrgManager(t.Context(), db, org.ID, reqFor(admin.ID.String()), "creating an agent"); res != nil {
		t.Fatalf("owner should be allowed: %v", res.Content)
	}

	// No actor (automated run) fails closed: these tools mutate org-wide agents
	// and must not be reachable from an externally-triggerable run.
	if res := requireOrgManager(t.Context(), db, org.ID, reqFor(""), "creating an agent"); res == nil {
		t.Fatal("automated run (no actor) must be blocked from creating agents")
	}

	// A present-but-non-member id is a hard, explicit error (never silently allowed).
	if res := requireOrgManager(t.Context(), db, org.ID, reqFor(uuid.NewString()), "creating an agent"); res == nil {
		t.Fatal("a non-member actor id must be rejected")
	}
}
