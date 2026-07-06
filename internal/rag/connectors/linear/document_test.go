package linear

import (
	"testing"
	"time"
)

// These literal strings are the prune contract — the scheduler diffs on
// them, so they are pinned exactly.
func TestDocIDFormat(t *testing.T) {
	if got := docIDForIssue("abc-123"); got != "linear_issue_abc-123" {
		t.Errorf("docIDForIssue = %q, want %q", got, "linear_issue_abc-123")
	}
	if got := docIDForProject("p-9"); got != "linear_project_p-9" {
		t.Errorf("docIDForProject = %q, want %q", got, "linear_project_p-9")
	}
	if got := docIDForInitiative("i-7"); got != "linear_initiative_i-7" {
		t.Errorf("docIDForInitiative = %q, want %q", got, "linear_initiative_i-7")
	}
}

func TestDocIDPureFunctionOfID(t *testing.T) {
	if docIDForIssue("a") == docIDForIssue("b") {
		t.Error("docIDForIssue collides across distinct ids")
	}
}

func fullIssue() linearIssue {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	iss := linearIssue{
		ID:            "uuid-1",
		Identifier:    "ENG-42",
		Title:         "Fix the widget",
		Description:   "the body",
		URL:           "https://linear.app/x/issue/ENG-42",
		CreatedAt:     created,
		UpdatedAt:     updated,
		PriorityLabel: "High",
		DueDate:       "2026-03-01",
		Team:          &linearTeam{ID: "t1", Name: "Engineering", Key: "ENG"},
		State:         &linearState{Name: "In Progress"},
		Assignee:      &linearUser{Name: "Alice", Email: "alice@x.com"},
		Creator:       &linearUser{Name: "Bob", Email: "bob@x.com"},
		Project:       &linearProjectRef{ID: "pr1", Name: "Q1 Roadmap"},
	}
	iss.Labels.Nodes = []linearLabel{{Name: "bug"}, {Name: "urgent"}}
	return iss
}

func TestIssueToDocumentFull(t *testing.T) {
	iss := fullIssue()
	comments := []linearComment{
		{ID: "c1", Body: "first comment", URL: "https://linear.app/c/1", User: &linearUser{Name: "Carol"}},
		{ID: "c2", Body: "second comment", URL: "", User: nil},
	}

	doc := issueToDocument(iss, comments)

	if doc.DocID != "linear_issue_uuid-1" {
		t.Errorf("DocID = %q", doc.DocID)
	}
	if doc.SemanticID != "[ENG-42] Fix the widget" {
		t.Errorf("SemanticID = %q", doc.SemanticID)
	}
	if doc.Link != iss.URL {
		t.Errorf("Link = %q", doc.Link)
	}
	if doc.DocUpdatedAt == nil || !doc.DocUpdatedAt.Equal(iss.UpdatedAt) {
		t.Errorf("DocUpdatedAt = %v", doc.DocUpdatedAt)
	}

	// description section + 2 comment sections, in order.
	if len(doc.Sections) != 3 {
		t.Fatalf("Sections len = %d, want 3", len(doc.Sections))
	}
	if doc.Sections[0].Text != "the body" || doc.Sections[0].Link != iss.URL {
		t.Errorf("description section = %+v", doc.Sections[0])
	}
	if doc.Sections[1].Text != "first comment" || doc.Sections[1].Link != "https://linear.app/c/1" {
		t.Errorf("comment 1 section = %+v", doc.Sections[1])
	}
	if doc.Sections[1].Title != "Comment by Carol" {
		t.Errorf("comment 1 title = %q", doc.Sections[1].Title)
	}
	// empty URL falls back to issue URL; nil user omits title.
	if doc.Sections[2].Link != iss.URL {
		t.Errorf("comment 2 link fallback = %q", doc.Sections[2].Link)
	}
	if doc.Sections[2].Title != "" {
		t.Errorf("comment 2 title = %q, want empty", doc.Sections[2].Title)
	}

	// owners: creator primary, assignee secondary, email preferred.
	if len(doc.PrimaryOwners) != 1 || doc.PrimaryOwners[0] != "bob@x.com" {
		t.Errorf("PrimaryOwners = %v", doc.PrimaryOwners)
	}
	if len(doc.SecondaryOwners) != 1 || doc.SecondaryOwners[0] != "alice@x.com" {
		t.Errorf("SecondaryOwners = %v", doc.SecondaryOwners)
	}

	want := map[string]string{
		"object_type": "Issue",
		"team":        "Engineering",
		"team_key":    "ENG",
		"state":       "In Progress",
		"assignee":    "Alice",
		"creator":     "Bob",
		"priority":    "High",
		"labels":      "bug,urgent",
		"project":     "Q1 Roadmap",
		"created_at":  "2026-01-02T03:04:05Z",
		"due_date":    "2026-03-01",
	}
	if len(doc.Metadata) != len(want) {
		t.Errorf("Metadata = %v, want %v", doc.Metadata, want)
	}
	for k, v := range want {
		if doc.Metadata[k] != v {
			t.Errorf("Metadata[%q] = %q, want %q", k, doc.Metadata[k], v)
		}
	}
}

func TestIssueToDocumentOwnerNameFallback(t *testing.T) {
	iss := fullIssue()
	iss.Creator = &linearUser{Name: "OnlyName"}
	iss.Assignee = &linearUser{Name: "AssignName"}
	doc := issueToDocument(iss, nil)
	if len(doc.PrimaryOwners) != 1 || doc.PrimaryOwners[0] != "OnlyName" {
		t.Errorf("PrimaryOwners fallback = %v", doc.PrimaryOwners)
	}
	if len(doc.SecondaryOwners) != 1 || doc.SecondaryOwners[0] != "AssignName" {
		t.Errorf("SecondaryOwners fallback = %v", doc.SecondaryOwners)
	}
}

func TestIssueToDocumentSparse(t *testing.T) {
	created := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	updated := time.Date(2026, 6, 7, 8, 9, 10, 0, time.UTC)
	iss := linearIssue{
		ID:         "sparse-1",
		Identifier: "ENG-1",
		Title:      "Bare",
		URL:        "https://linear.app/x/issue/ENG-1",
		CreatedAt:  created,
		UpdatedAt:  updated,
	}

	doc := issueToDocument(iss, nil) // no panics

	// only the description section remains.
	if len(doc.Sections) != 1 {
		t.Fatalf("Sections len = %d, want 1", len(doc.Sections))
	}
	if doc.PrimaryOwners != nil {
		t.Errorf("PrimaryOwners = %v, want nil", doc.PrimaryOwners)
	}
	if doc.SecondaryOwners != nil {
		t.Errorf("SecondaryOwners = %v, want nil", doc.SecondaryOwners)
	}

	absent := []string{"team", "team_key", "state", "assignee", "creator", "priority", "labels", "project", "due_date"}
	for _, k := range absent {
		if _, ok := doc.Metadata[k]; ok {
			t.Errorf("Metadata[%q] present, want absent", k)
		}
	}
	if doc.Metadata["object_type"] != "Issue" {
		t.Errorf("object_type = %q", doc.Metadata["object_type"])
	}
	// created_at is still present since CreatedAt is set.
	if doc.Metadata["created_at"] != "2026-05-06T07:08:09Z" {
		t.Errorf("created_at = %q", doc.Metadata["created_at"])
	}
}

func TestProjectToDocument(t *testing.T) {
	updated := time.Date(2026, 4, 4, 4, 4, 4, 0, time.UTC)
	p := linearProject{
		ID:          "proj-1",
		Name:        "Roadmap",
		Description: "short desc",
		Content:     "# long body",
		URL:         "https://linear.app/x/project/roadmap",
		UpdatedAt:   updated,
	}
	p.Initiatives.Nodes = []linearInitiative{{Name: "Growth"}, {Name: "Retention"}}

	doc := projectToDocument(p)

	if doc.DocID != "linear_project_proj-1" {
		t.Errorf("DocID = %q", doc.DocID)
	}
	if doc.SemanticID != "Roadmap" {
		t.Errorf("SemanticID = %q", doc.SemanticID)
	}
	if len(doc.Sections) != 2 || doc.Sections[0].Text != "short desc" || doc.Sections[1].Text != "# long body" {
		t.Errorf("Sections = %+v", doc.Sections)
	}
	if doc.Metadata["object_type"] != "Project" {
		t.Errorf("object_type = %q", doc.Metadata["object_type"])
	}
	if doc.Metadata["initiatives"] != "Growth,Retention" {
		t.Errorf("initiatives = %q", doc.Metadata["initiatives"])
	}
}

func TestProjectToDocumentEmptyContent(t *testing.T) {
	p := linearProject{ID: "proj-2", Name: "Empty", Description: "d", URL: "u"}
	doc := projectToDocument(p)
	if len(doc.Sections) != 2 || doc.Sections[1].Text != "" {
		t.Errorf("Sections = %+v", doc.Sections)
	}
	if _, ok := doc.Metadata["initiatives"]; ok {
		t.Error("initiatives present, want absent for no initiatives")
	}
}

func TestInitiativeToDocument(t *testing.T) {
	updated := time.Date(2026, 7, 7, 7, 7, 7, 0, time.UTC)
	i := linearInitiative{
		ID:          "init-1",
		Name:        "Growth",
		Description: "grow it",
		Content:     "body md",
		URL:         "https://linear.app/x/initiative/growth",
		UpdatedAt:   updated,
	}
	doc := initiativeToDocument(i)
	if doc.DocID != "linear_initiative_init-1" {
		t.Errorf("DocID = %q", doc.DocID)
	}
	if doc.SemanticID != "Growth" {
		t.Errorf("SemanticID = %q", doc.SemanticID)
	}
	if len(doc.Sections) != 2 || doc.Sections[0].Text != "grow it" || doc.Sections[1].Text != "body md" {
		t.Errorf("Sections = %+v", doc.Sections)
	}
	if doc.Metadata["object_type"] != "Initiative" {
		t.Errorf("object_type = %q", doc.Metadata["object_type"])
	}
}

func TestInitiativeToDocumentEmptyContent(t *testing.T) {
	i := linearInitiative{ID: "init-2", Name: "N", Description: "d", URL: "u"}
	doc := initiativeToDocument(i)
	if len(doc.Sections) != 2 || doc.Sections[1].Text != "" {
		t.Errorf("Sections = %+v", doc.Sections)
	}
}
