package github

import "time"

type GithubUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Email string `json:"email,omitempty"`
	Type  string `json:"type,omitempty"`
}

// GithubPRRef on an issue response means the row is a PR; the fetch
// loop skips it.
type GithubPRRef struct {
	URL string `json:"url,omitempty"`
}

type GithubPR struct {
	ID        int64         `json:"id"`
	Number    int           `json:"number"`
	Title     string        `json:"title"`
	Body      string        `json:"body"`
	State     string        `json:"state"`
	HTMLURL   string        `json:"html_url"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	MergedAt  *time.Time    `json:"merged_at,omitempty"`
	User      *GithubUser   `json:"user,omitempty"`
	Assignees []GithubUser  `json:"assignees,omitempty"`
	Labels    []GithubLabel `json:"labels,omitempty"`
}

type GithubIssue struct {
	ID          int64         `json:"id"`
	Number      int           `json:"number"`
	Title       string        `json:"title"`
	Body        string        `json:"body"`
	State       string        `json:"state"`
	HTMLURL     string        `json:"html_url"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	User        *GithubUser   `json:"user,omitempty"`
	Assignees   []GithubUser  `json:"assignees,omitempty"`
	Labels      []GithubLabel `json:"labels,omitempty"`
	PullRequest *GithubPRRef  `json:"pull_request,omitempty"`
}

type GithubLabel struct {
	Name string `json:"name"`
}
