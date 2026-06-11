package github

import (
	"context"

	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// mapVisibility produces the same group IDs SyncExternalGroups emits
// for the same repo — both sides must agree byte-for-byte or the
// per-doc ACL won't match the published group catalog.
func mapVisibility(repo GithubRepo) *interfaces.ExternalAccess {
	if isPublic(repo) {
		return &interfaces.ExternalAccess{IsPublic: true}
	}
	if isInternal(repo) {
		return &interfaces.ExternalAccess{
			ExternalUserGroupIDs: []string{orgGroupID(repo.Owner.ID)},
		}
	}
	return &interfaces.ExternalAccess{
		ExternalUserGroupIDs: []string{
			collaboratorsGroupID(repo.ID),
			outsideCollaboratorsGroupID(repo.ID),
		},
	}
}

func (c *GithubConnector) SyncDocPermissions(
	ctx context.Context, _ interfaces.Source,
) (<-chan interfaces.DocExternalAccessOrFailure, error) {
	out := make(chan interfaces.DocExternalAccessOrFailure, c.channelBuf)
	goroutine.Go(ctx, func(ctx context.Context) {
		defer close(out)
		for _, full := range c.repoFullNames() {
			repo, err := c.client.getRepo(ctx, full)
			if err != nil {
				if !interfaces.Send(ctx, out, interfaces.NewAccessFailure(entityFailure(full, "github: get repo", err))) {
					return
				}
				continue
			}
			access := mapVisibility(repo)
			if !c.streamRepoDocAccess(ctx, full, access, out) {
				return
			}
		}
	})
	return out, nil
}

func (c *GithubConnector) streamRepoDocAccess(
	ctx context.Context, fullName string, access *interfaces.ExternalAccess,
	out chan<- interfaces.DocExternalAccessOrFailure,
) bool {
	if c.cfg.IncludePRs {
		page := 1
		for {
			prs, next, err := c.client.listPullRequestsPage(ctx, fullName, "all", page)
			if err != nil {
				if !interfaces.Send(ctx, out, interfaces.NewAccessFailure(entityFailure(fullName, "github: list PRs", err))) {
					return false
				}
				break
			}
			for _, pr := range prs {
				if !interfaces.Send(ctx, out, interfaces.NewAccessResult(&interfaces.DocExternalAccess{
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
				if !interfaces.Send(ctx, out, interfaces.NewAccessFailure(entityFailure(fullName, "github: list issues", err))) {
					return false
				}
				break
			}
			for _, issue := range issues {
				if issue.PullRequest != nil {
					continue
				}
				if !interfaces.Send(ctx, out, interfaces.NewAccessResult(&interfaces.DocExternalAccess{
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

func (c *GithubConnector) SyncExternalGroups(
	ctx context.Context, _ interfaces.Source,
) (<-chan interfaces.ExternalGroupOrFailure, error) {
	out := make(chan interfaces.ExternalGroupOrFailure, c.channelBuf)
	goroutine.Go(ctx, func(ctx context.Context) {
		defer close(out)
		for _, full := range c.repoFullNames() {
			repo, err := c.client.getRepo(ctx, full)
			if err != nil {
				if !interfaces.Send(ctx, out, interfaces.NewGroupFailure(entityFailure(full, "github: get repo", err))) {
					return
				}
				continue
			}
			if !c.enumerateGroups(ctx, repo, out) {
				return
			}
		}
	})
	return out, nil
}

// A partial failure (one team's members fail) does not kill the whole
// sync — each helper wraps its own error path. Returns false when ctx
// was cancelled (the caller must stop producing).
func (c *GithubConnector) enumerateGroups(
	ctx context.Context, repo GithubRepo, out chan<- interfaces.ExternalGroupOrFailure,
) bool {
	if isPublic(repo) {
		return true
	}
	if isInternal(repo) {
		members, err := c.client.listOrgMembers(ctx, repo.Owner.Login)
		if err != nil {
			return interfaces.Send(ctx, out, interfaces.NewGroupFailure(entityFailure(repo.FullName, "github: list org members", err)))
		}
		return interfaces.Send(ctx, out, interfaces.NewGroupResult(&interfaces.ExternalGroup{
			GroupID:      orgGroupID(repo.Owner.ID),
			DisplayName:  repo.Owner.Login + " organization",
			MemberEmails: emailsFromMemberships(members),
		}))
	}
	if !c.emitCollaboratorGroups(ctx, repo, out) {
		return false
	}
	return c.emitTeamGroups(ctx, repo, out)
}

func (c *GithubConnector) emitCollaboratorGroups(
	ctx context.Context, repo GithubRepo, out chan<- interfaces.ExternalGroupOrFailure,
) bool {
	direct, err := c.client.listCollaborators(ctx, repo.FullName, "direct")
	if err != nil {
		if !interfaces.Send(ctx, out, interfaces.NewGroupFailure(entityFailure(repo.FullName, "github: direct collaborators", err))) {
			return false
		}
	} else {
		if !interfaces.Send(ctx, out, interfaces.NewGroupResult(&interfaces.ExternalGroup{
			GroupID:      collaboratorsGroupID(repo.ID),
			DisplayName:  repo.FullName + " collaborators",
			MemberEmails: emailsFromUsers(direct),
		})) {
			return false
		}
	}
	outside, err := c.client.listCollaborators(ctx, repo.FullName, "outside")
	if err != nil {
		return interfaces.Send(ctx, out, interfaces.NewGroupFailure(entityFailure(repo.FullName, "github: outside collaborators", err)))
	}
	return interfaces.Send(ctx, out, interfaces.NewGroupResult(&interfaces.ExternalGroup{
		GroupID:      outsideCollaboratorsGroupID(repo.ID),
		DisplayName:  repo.FullName + " outside collaborators",
		MemberEmails: emailsFromUsers(outside),
	}))
}

func (c *GithubConnector) emitTeamGroups(
	ctx context.Context, repo GithubRepo, out chan<- interfaces.ExternalGroupOrFailure,
) bool {
	teams, err := c.client.listTeams(ctx, repo.FullName)
	if err != nil {
		return interfaces.Send(ctx, out, interfaces.NewGroupFailure(entityFailure(repo.FullName, "github: teams", err)))
	}
	for _, t := range teams {
		members, err := c.client.listTeamMembers(ctx, t.ID)
		if err != nil {
			if !interfaces.Send(ctx, out, interfaces.NewGroupFailure(entityFailure(repo.FullName, "github: team members "+t.Slug, err))) {
				return false
			}
			continue
		}
		if !interfaces.Send(ctx, out, interfaces.NewGroupResult(&interfaces.ExternalGroup{
			GroupID:      teamGroupID(t.Slug),
			DisplayName:  t.Name,
			MemberEmails: emailsFromUsers(members),
		})) {
			return false
		}
	}
	return true
}
