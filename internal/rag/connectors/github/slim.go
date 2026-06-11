package github

import (
	"context"

	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

func (c *GithubConnector) ListAllSlim(
	ctx context.Context, _ interfaces.Source,
) (<-chan interfaces.SlimDocOrFailure, error) {
	out := make(chan interfaces.SlimDocOrFailure, c.channelBuf)
	goroutine.Go(ctx, func(ctx context.Context) {
		defer close(out)
		for _, full := range c.repoFullNames() {
			repo, err := c.client.getRepo(ctx, full)
			if err != nil {
				if !interfaces.Send(ctx, out, interfaces.NewSlimFailure(entityFailure(full, "github: get repo for slim", err))) {
					return
				}
				continue
			}
			access := mapVisibility(repo)
			if !c.streamRepoSlim(ctx, full, access, out) {
				return
			}
		}
	})
	return out, nil
}

// Reuses the same listing endpoints as ingest so the prune diff sees
// the exact same doc-id alphabet. Returns false when ctx was cancelled
// (the caller must stop producing).
func (c *GithubConnector) streamRepoSlim(
	ctx context.Context, fullName string, access *interfaces.ExternalAccess,
	out chan<- interfaces.SlimDocOrFailure,
) bool {
	if c.cfg.IncludePRs {
		page := 1
		for {
			prs, next, err := c.client.listPullRequestsPage(ctx, fullName, "all", page)
			if err != nil {
				if !interfaces.Send(ctx, out, interfaces.NewSlimFailure(entityFailure(fullName, "github: list PRs slim", err))) {
					return false
				}
				break
			}
			for _, pr := range prs {
				if !interfaces.Send(ctx, out, interfaces.NewSlimResult(&interfaces.SlimDocument{
					DocID:          docIDForPR(fullName, pr),
					ExternalAccess: access,
				})) {
					return false
				}
			}
			if next == 0 {
				break
			}
			page = next
		}
	}
	if c.cfg.IncludeIssues {
		page := 1
		for {
			issues, next, err := c.client.listIssuesPage(ctx, fullName, "all", page)
			if err != nil {
				if !interfaces.Send(ctx, out, interfaces.NewSlimFailure(entityFailure(fullName, "github: list issues slim", err))) {
					return false
				}
				break
			}
			for _, issue := range issues {
				if issue.PullRequest != nil {
					continue
				}
				if !interfaces.Send(ctx, out, interfaces.NewSlimResult(&interfaces.SlimDocument{
					DocID:          docIDForIssue(fullName, issue),
					ExternalAccess: access,
				})) {
					return false
				}
			}
			if next == 0 {
				break
			}
			page = next
		}
	}
	return true
}
