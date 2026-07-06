package linear

import (
	"strings"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// docIDForIssue is a pure function of the Linear node UUID so the same
// issue keys identically across ingest and the scheduler's prune diff.
func docIDForIssue(id string) string {
	return "linear_issue_" + id
}

func docIDForProject(id string) string {
	return "linear_project_" + id
}

func docIDForInitiative(id string) string {
	return "linear_initiative_" + id
}

// issueToDocument renders an issue and its fully-resolved comment list into
// the neutral Document shape. comments is the caller-merged inline-first-page
// plus paginated remainder — issue.Comments.Nodes is deliberately NOT read
// here.
func issueToDocument(issue linearIssue, comments []linearComment) interfaces.Document {
	updatedAt := issue.UpdatedAt

	sections := make([]interfaces.Section, 0, len(comments)+1)
	sections = append(sections, interfaces.Section{Text: issue.Description, Link: issue.URL})
	for _, c := range comments {
		link := c.URL
		if link == "" {
			link = issue.URL
		}
		title := ""
		if c.User != nil && c.User.Name != "" {
			title = "Comment by " + c.User.Name
		}
		sections = append(sections, interfaces.Section{Text: c.Body, Link: link, Title: title})
	}

	return interfaces.Document{
		DocID:           docIDForIssue(issue.ID),
		SemanticID:      "[" + issue.Identifier + "] " + issue.Title,
		Link:            issue.URL,
		Sections:        sections,
		DocUpdatedAt:    &updatedAt,
		PrimaryOwners:   userEmails(issue.Creator),
		SecondaryOwners: userEmails(issue.Assignee),
		Metadata:        issueMetadata(issue),
	}
}

// projectToDocument maps a project (with its long-form Content body) into a
// Document. Description and Content each become their own section.
func projectToDocument(p linearProject) interfaces.Document {
	updatedAt := p.UpdatedAt
	m := map[string]string{"object_type": "Project"}
	if names := joinInitiatives(p.Initiatives.Nodes); names != "" {
		m["initiatives"] = names
	}
	return interfaces.Document{
		DocID:      docIDForProject(p.ID),
		SemanticID: p.Name,
		Link:       p.URL,
		Sections: []interfaces.Section{
			{Text: p.Description, Link: p.URL},
			{Text: p.Content, Link: p.URL},
		},
		DocUpdatedAt: &updatedAt,
		Metadata:     m,
	}
}

// initiativeToDocument maps an initiative into a Document, mirroring the
// project layout (Description + Content sections).
func initiativeToDocument(i linearInitiative) interfaces.Document {
	updatedAt := i.UpdatedAt
	return interfaces.Document{
		DocID:      docIDForInitiative(i.ID),
		SemanticID: i.Name,
		Link:       i.URL,
		Sections: []interfaces.Section{
			{Text: i.Description, Link: i.URL},
			{Text: i.Content, Link: i.URL},
		},
		DocUpdatedAt: &updatedAt,
		Metadata:     map[string]string{"object_type": "Initiative"},
	}
}

// userEmails falls back to name when email is absent so downstream
// consumers always see a non-empty owner list, mirroring github ownerEmails.
func userEmails(u *linearUser) []string {
	if u == nil {
		return nil
	}
	if u.Email != "" {
		return []string{u.Email}
	}
	if u.Name != "" {
		return []string{u.Name}
	}
	return nil
}

func issueMetadata(issue linearIssue) map[string]string {
	m := map[string]string{"object_type": "Issue"}
	if issue.Team != nil {
		setNonEmpty(m, "team", issue.Team.Name)
		setNonEmpty(m, "team_key", issue.Team.Key)
	}
	if issue.State != nil {
		setNonEmpty(m, "state", issue.State.Name)
	}
	if issue.Assignee != nil {
		setNonEmpty(m, "assignee", issue.Assignee.Name)
	}
	if issue.Creator != nil {
		setNonEmpty(m, "creator", issue.Creator.Name)
	}
	setNonEmpty(m, "priority", issue.PriorityLabel)
	setNonEmpty(m, "labels", joinLabels(issue.Labels.Nodes))
	if issue.Project != nil {
		setNonEmpty(m, "project", issue.Project.Name)
	}
	if !issue.CreatedAt.IsZero() {
		setNonEmpty(m, "created_at", issue.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"))
	}
	setNonEmpty(m, "due_date", issue.DueDate)
	return m
}

func setNonEmpty(m map[string]string, key, value string) {
	if value != "" {
		m[key] = value
	}
}

// joinLabels comma-joins label names into a single Metadata value, since
// Document.Metadata is string→string by contract (github joinLabels-style).
func joinLabels(labels []linearLabel) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(labels))
	for _, l := range labels {
		if l.Name != "" {
			parts = append(parts, l.Name)
		}
	}
	return strings.Join(parts, ",")
}

func joinInitiatives(initiatives []linearInitiative) string {
	if len(initiatives) == 0 {
		return ""
	}
	parts := make([]string, 0, len(initiatives))
	for _, i := range initiatives {
		if i.Name != "" {
			parts = append(parts, i.Name)
		}
	}
	return strings.Join(parts, ",")
}
