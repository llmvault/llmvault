package linear

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Client is the typed Linear GraphQL surface the connector drives. Every
// method wraps a single logical request in withRetry, unwraps the
// `data.<field>` envelope, and returns the contract types. sleep and now
// are fields so tests can inject a recorder / fixed clock.
type Client struct {
	p     proxyClient
	sleep func(context.Context, time.Duration) error
	now   func() time.Time
}

func newClient(p proxyClient) *Client {
	return &Client{
		p:     p,
		sleep: defaultSleep,
		now:   time.Now,
	}
}

// do performs one GraphQL request with retries and returns the `data`
// object. Throttles wait+retry, 5xx/network errors back off and retry,
// and GraphQL errors / non-throttle 4xx / decode failures are permanent.
func (c *Client) do(ctx context.Context, opName, query string, vars map[string]any) (json.RawMessage, error) {
	body, err := graphQLBody(opName, query, vars)
	if err != nil {
		return nil, fmt.Errorf("linear %s: build request: %w", opName, err)
	}

	var data json.RawMessage
	err = c.withRetry(ctx, func() error {
		status, hdr, respBody, e := c.p.Do(ctx, body)
		if e != nil {
			return e // network failure — transient
		}
		if wait, ok := detectRateLimit(status, hdr, respBody, c.now()); ok {
			return &rateLimitError{wait: wait}
		}
		if status >= 500 {
			return fmt.Errorf("linear %s: status %d: %s", opName, status, preview(respBody))
		}
		var resp graphQLResponse
		if e := json.Unmarshal(respBody, &resp); e != nil {
			return &permanentError{fmt.Errorf("linear %s: decode: %w", opName, e)}
		}
		if len(resp.Errors) > 0 {
			return &permanentError{fmt.Errorf("linear %s: %s", opName, joinErrors(resp.Errors))}
		}
		if status >= 400 {
			return &permanentError{fmt.Errorf("linear %s: status %d: %s", opName, status, preview(respBody))}
		}
		data = resp.Data
		return nil
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}

func preview(b []byte) string {
	if len(b) > 256 {
		b = b[:256]
	}
	return string(b)
}

// Teams lists every team, paginating internally at 100 per page.
func (c *Client) Teams(ctx context.Context) ([]linearTeam, error) {
	var out []linearTeam
	cursor := ""
	for {
		vars := map[string]any{}
		if cursor != "" {
			vars["cursor"] = cursor
		}
		data, err := c.do(ctx, "Teams", teamsQuery, vars)
		if err != nil {
			return nil, err
		}
		var resp struct {
			Teams struct {
				Nodes    []linearTeam `json:"nodes"`
				PageInfo pageInfo     `json:"pageInfo"`
			} `json:"teams"`
		}
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("linear Teams: decode data: %w", err)
		}
		out = append(out, resp.Teams.Nodes...)
		if !resp.Teams.PageInfo.HasNextPage || resp.Teams.PageInfo.EndCursor == "" {
			return out, nil
		}
		cursor = resp.Teams.PageInfo.EndCursor
	}
}

// TeamProjects returns one page (50) of a team's projects with the first
// 50 initiatives nested on each. The caller drives pagination via the
// returned pageInfo.
func (c *Client) TeamProjects(ctx context.Context, teamID, cursor string) ([]linearProject, pageInfo, error) {
	vars := map[string]any{"teamId": teamID}
	if cursor != "" {
		vars["cursor"] = cursor
	}
	data, err := c.do(ctx, "TeamProjects", teamProjectsQuery, vars)
	if err != nil {
		return nil, pageInfo{}, err
	}
	var resp struct {
		Team struct {
			Projects struct {
				Nodes    []linearProject `json:"nodes"`
				PageInfo pageInfo        `json:"pageInfo"`
			} `json:"projects"`
		} `json:"team"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, pageInfo{}, fmt.Errorf("linear TeamProjects: decode data: %w", err)
	}
	return resp.Team.Projects.Nodes, resp.Team.Projects.PageInfo, nil
}

// TeamIssues returns one page (50) of issues across the given teams. When
// updatedSince is non-nil the query filters incrementally (issue OR any
// comment updated since). The caller drives pagination via pageInfo.
func (c *Client) TeamIssues(
	ctx context.Context, teamIDs []string, updatedSince *time.Time, cursor string,
) ([]linearIssue, pageInfo, error) {
	vars := map[string]any{"teamIds": teamIDs}
	if cursor != "" {
		vars["cursor"] = cursor
	}
	if updatedSince != nil {
		vars["since"] = updatedSince.UTC().Format(time.RFC3339)
	}
	data, err := c.do(ctx, "TeamIssues", buildTeamIssuesQuery(updatedSince != nil), vars)
	if err != nil {
		return nil, pageInfo{}, err
	}
	var resp struct {
		Issues struct {
			Nodes    []linearIssue `json:"nodes"`
			PageInfo pageInfo      `json:"pageInfo"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, pageInfo{}, fmt.Errorf("linear TeamIssues: decode data: %w", err)
	}
	return resp.Issues.Nodes, resp.Issues.PageInfo, nil
}

// IssueComments returns one page (50) of a single issue's comments, used
// to drain issues whose nested comment page overflowed.
func (c *Client) IssueComments(ctx context.Context, issueID, cursor string) ([]linearComment, pageInfo, error) {
	vars := map[string]any{"id": issueID}
	if cursor != "" {
		vars["cursor"] = cursor
	}
	data, err := c.do(ctx, "IssueComments", issueCommentsQuery, vars)
	if err != nil {
		return nil, pageInfo{}, err
	}
	var resp struct {
		Issue struct {
			Comments struct {
				Nodes    []linearComment `json:"nodes"`
				PageInfo pageInfo        `json:"pageInfo"`
			} `json:"comments"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, pageInfo{}, fmt.Errorf("linear IssueComments: decode data: %w", err)
	}
	return resp.Issue.Comments.Nodes, resp.Issue.Comments.PageInfo, nil
}

// SlimTeamIssues returns one page (100) of issue ids across the given
// teams (no updatedAt filter). The caller drives pagination via pageInfo.
func (c *Client) SlimTeamIssues(ctx context.Context, teamIDs []string, cursor string) ([]slimIssue, pageInfo, error) {
	vars := map[string]any{"teamIds": teamIDs}
	if cursor != "" {
		vars["cursor"] = cursor
	}
	data, err := c.do(ctx, "SlimTeamIssues", slimTeamIssuesQuery, vars)
	if err != nil {
		return nil, pageInfo{}, err
	}
	var resp struct {
		Issues struct {
			Nodes    []slimIssue `json:"nodes"`
			PageInfo pageInfo    `json:"pageInfo"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, pageInfo{}, fmt.Errorf("linear SlimTeamIssues: decode data: %w", err)
	}
	return resp.Issues.Nodes, resp.Issues.PageInfo, nil
}
