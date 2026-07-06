package linear

import (
	"encoding/json"
	"time"
)

// pageInfo is the standard Relay-style cursor block present on every
// Linear GraphQL connection.
type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// linearUser is a minimal actor reference (assignee, creator, comment
// author).
type linearUser struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// linearTeam is a team; Key is the short prefix used in issue
// identifiers (e.g. "ENG").
type linearTeam struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"`
}

// linearState is a workflow state (backlog, in-progress, done, ...).
type linearState struct {
	Name string `json:"name"`
}

// linearLabel is an issue label.
type linearLabel struct {
	Name string `json:"name"`
}

// linearComment is one comment on an issue.
type linearComment struct {
	ID        string      `json:"id"`
	Body      string      `json:"body"`
	URL       string      `json:"url"`
	UpdatedAt time.Time   `json:"updatedAt"`
	User      *linearUser `json:"user"`
}

// linearInitiative is an initiative a project rolls up into.
type linearInitiative struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Content     string    `json:"content"`
	URL         string    `json:"url"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// linearProject is a project with its nested initiatives (first page).
type linearProject struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Content     string    `json:"content"`
	URL         string    `json:"url"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Initiatives struct {
		Nodes []linearInitiative `json:"nodes"`
	} `json:"initiatives"`
}

// linearProjectRef is the trimmed project reference embedded on an issue.
type linearProjectRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// linearIssue is an issue with its nested labels and first page of
// comments.
type linearIssue struct {
	ID            string    `json:"id"`
	Identifier    string    `json:"identifier"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	URL           string    `json:"url"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	PriorityLabel string    `json:"priorityLabel"`
	DueDate       string    `json:"dueDate"`

	Team     *linearTeam       `json:"team"`
	State    *linearState      `json:"state"`
	Assignee *linearUser       `json:"assignee"`
	Creator  *linearUser       `json:"creator"`
	Project  *linearProjectRef `json:"project"`

	Labels struct {
		Nodes []linearLabel `json:"nodes"`
	} `json:"labels"`
	Comments struct {
		Nodes    []linearComment `json:"nodes"`
		PageInfo pageInfo        `json:"pageInfo"`
	} `json:"comments"`
}

// slimIssue is the id-only projection used by the pruning loop.
type slimIssue struct {
	ID string `json:"id"`
}

// graphQLResponse is the envelope every Linear GraphQL response arrives
// in: a `data` object and/or a list of `errors`.
type graphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphQLError  `json:"errors"`
}

// graphQLError is one entry in a GraphQL response's `errors` array. The
// extensions.code carries "RATELIMITED" when Linear throttles a request.
type graphQLError struct {
	Message    string `json:"message"`
	Extensions struct {
		Code string `json:"code"`
	} `json:"extensions"`
}
