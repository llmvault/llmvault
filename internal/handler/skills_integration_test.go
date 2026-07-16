package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// TestSkillHTTPCRUDAndTeamIsolation exercises the real handler, access
// resolver, and Postgres constraints for member-owned team skills and
// admin-owned org skills.
func TestSkillHTTPCRUDAndTeamIsolation(t *testing.T) {
	db := connectTestDB(t)
	org := model.Org{ID: uuid.New(), Name: "skill-http-" + uuid.NewString()[:8], Active: true}
	member := model.User{ID: uuid.New(), Email: "skill-member-" + uuid.NewString() + "@example.test"}
	admin := model.User{ID: uuid.New(), Email: "skill-admin-" + uuid.NewString() + "@example.test"}
	teamA := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "Skill Team A " + uuid.NewString()[:5]}
	teamB := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "Skill Team B " + uuid.NewString()[:5]}
	rows := []any{
		&org, &member, &admin, &teamA, &teamB,
		&model.OrgMembership{ID: uuid.New(), OrgID: org.ID, UserID: member.ID, Role: "member"},
		&model.OrgMembership{ID: uuid.New(), OrgID: org.ID, UserID: admin.ID, Role: "admin"},
		&model.TeamMember{ID: uuid.New(), OrgID: org.ID, TeamID: teamA.ID, UserID: member.ID, Role: "member"},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}
	foreignTeamSkill := model.Skill{
		ID: uuid.New(), OrgID: &org.ID, TeamID: &teamB.ID,
		Slug: "foreign-" + uuid.NewString()[:8], Name: "Foreign team skill",
		SourceType: model.SkillSourceInline, Bundle: model.RawJSON(`{}`), Status: model.SkillStatusPublished,
	}
	for _, row := range []*model.Skill{&foreignTeamSkill} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed skill: %v", err)
		}
	}

	router := chi.NewRouter()
	handler.NewSkillHandler(db).Mount(router)
	do := func(user *model.User, method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var payload bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&payload).Encode(body); err != nil {
				t.Fatalf("encode request: %v", err)
			}
		}
		req := httptest.NewRequest(method, path, &payload)
		req.Header.Set("Content-Type", "application/json")
		req = middleware.WithOrg(req, &org)
		req = middleware.WithUser(req, user)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}

	createTeam := do(&member, http.MethodPost, "/skills", map[string]any{
		"team_id": teamA.ID.String(), "slug": "owned-" + uuid.NewString()[:8],
		"name": "Owned skill", "bundle": map[string]any{"content": "team instructions"},
	})
	if createTeam.Code != http.StatusCreated {
		t.Fatalf("member create team skill status=%d body=%s", createTeam.Code, createTeam.Body.String())
	}
	var created struct {
		Skill struct {
			ID     string  `json:"id"`
			TeamID *string `json:"team_id"`
		} `json:"skill"`
	}
	if err := json.NewDecoder(createTeam.Body).Decode(&created); err != nil {
		t.Fatalf("decode created skill: %v", err)
	}
	if created.Skill.TeamID == nil || *created.Skill.TeamID != teamA.ID.String() {
		t.Fatalf("created skill team = %#v, want %s", created.Skill.TeamID, teamA.ID)
	}

	memberOrgCreate := do(&member, http.MethodPost, "/skills", map[string]any{
		"slug": "forbidden-" + uuid.NewString()[:8], "name": "Forbidden", "bundle": map[string]any{},
	})
	if memberOrgCreate.Code != http.StatusForbidden {
		t.Fatalf("member org skill status=%d body=%s", memberOrgCreate.Code, memberOrgCreate.Body.String())
	}

	updateOwned := do(&member, http.MethodPatch, "/skills/"+created.Skill.ID, map[string]any{"name": "Updated owned skill"})
	if updateOwned.Code != http.StatusOK {
		t.Fatalf("member update owned status=%d body=%s", updateOwned.Code, updateOwned.Body.String())
	}
	updateForeign := do(&member, http.MethodPatch, "/skills/"+foreignTeamSkill.ID.String(), map[string]any{"name": "Leaked"})
	if updateForeign.Code != http.StatusNotFound {
		t.Fatalf("member update foreign status=%d body=%s", updateForeign.Code, updateForeign.Body.String())
	}

	createOrg := do(&admin, http.MethodPost, "/skills", map[string]any{
		"slug": "org-http-" + uuid.NewString()[:8], "name": "Org skill", "bundle": map[string]any{"content": "shared"},
	})
	if createOrg.Code != http.StatusCreated {
		t.Fatalf("admin create org skill status=%d body=%s", createOrg.Code, createOrg.Body.String())
	}
	var orgCreated struct {
		Skill struct {
			ID     string  `json:"id"`
			TeamID *string `json:"team_id"`
		} `json:"skill"`
	}
	if err := json.NewDecoder(createOrg.Body).Decode(&orgCreated); err != nil {
		t.Fatalf("decode org skill: %v", err)
	}
	if orgCreated.Skill.TeamID != nil {
		t.Fatalf("org skill unexpectedly team-scoped: %#v", orgCreated.Skill.TeamID)
	}
	archiveOrg := do(&admin, http.MethodDelete, "/skills/"+orgCreated.Skill.ID, nil)
	if archiveOrg.Code != http.StatusOK {
		t.Fatalf("admin archive org skill status=%d body=%s", archiveOrg.Code, archiveOrg.Body.String())
	}

	list := do(&member, http.MethodGet, "/skills", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("member list status=%d body=%s", list.Code, list.Body.String())
	}
	var listed struct {
		Skills []struct {
			ID string `json:"id"`
		} `json:"skills"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	seen := map[string]bool{}
	for _, skill := range listed.Skills {
		seen[skill.ID] = true
	}
	if !seen[created.Skill.ID] {
		t.Fatalf("member list omitted owned skill: %#v", seen)
	}
	if seen[foreignTeamSkill.ID.String()] || seen[orgCreated.Skill.ID] {
		t.Fatalf("member list leaked foreign or archived skill: %#v", seen)
	}
}
