package github

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// pageSize is the GitHub API maximum on list endpoints.
const pageSize = 100

type Client struct {
	p proxyClient
}

func newClient(p proxyClient) *Client {
	return &Client{p: p}
}

func (c *Client) listPullRequestsPage(
	ctx context.Context, fullName, state string, page int,
) ([]GithubPR, int, error) {
	q := url.Values{}
	q.Set("state", state)
	q.Set("sort", "updated")
	q.Set("direction", "desc")
	q.Set("per_page", strconv.Itoa(pageSize))
	q.Set("page", strconv.Itoa(page))

	var prs []GithubPR
	var hdr http.Header
	err := withRateLimitRetry(ctx, func() error {
		got, h, err := getJSON[[]GithubPR](ctx, c.p, http.MethodGet,
			"/repos/"+fullName+"/pulls", q)
		if err != nil {
			return err
		}
		prs = got
		hdr = h
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	next, _ := nextPageNumber(hdr)
	return prs, next, nil
}

// listIssuesPage: GitHub returns PRs as issues; the caller filters
// out entries with PullRequest != nil.
func (c *Client) listIssuesPage(
	ctx context.Context, fullName, state string, page int,
) ([]GithubIssue, int, error) {
	q := url.Values{}
	q.Set("state", state)
	q.Set("sort", "updated")
	q.Set("direction", "desc")
	q.Set("per_page", strconv.Itoa(pageSize))
	q.Set("page", strconv.Itoa(page))

	var issues []GithubIssue
	var hdr http.Header
	err := withRateLimitRetry(ctx, func() error {
		got, h, err := getJSON[[]GithubIssue](ctx, c.p, http.MethodGet,
			"/repos/"+fullName+"/issues", q)
		if err != nil {
			return err
		}
		issues = got
		hdr = h
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	next, _ := nextPageNumber(hdr)
	return issues, next, nil
}
