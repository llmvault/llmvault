package linear

import (
	"context"
	"sort"
	"time"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// issuesEntityID labels the combined multi-team issue walk in entity
// failures, since that walk isn't attributable to a single team.
const issuesEntityID = "linear_issues"

// run drives the staged walk: PROJECTS (per-team project + collected
// initiative docs) then ISSUES (one combined cursor walk over issues
// across all teams). The deferred store persists whatever progress the
// checkpoint reached, even on early return / cancellation.
func (c *LinearConnector) run(
	ctx context.Context,
	cp LinearCheckpoint,
	updatedSince *time.Time,
	out chan<- interfaces.DocumentOrFailure,
) {
	defer close(out)
	defer func() {
		final := cp
		c.finalCp.Store(&final)
	}()

	// Initiative dedup is in-memory only. A run that resumes mid-PROJECTS
	// re-emits initiatives it already emitted before the interruption;
	// that's fine — upserts are idempotent.
	initiatives := make(map[string]linearInitiative)

	if cp.Stage == StageProjects {
		if !c.runProjects(ctx, &cp, initiatives, out) {
			return
		}
	}
	if cp.Stage == StageIssues {
		c.runIssues(ctx, &cp, updatedSince, out)
	}
}

// runProjects pages each pending team's projects, emitting a document
// per project and collecting deduped initiatives (first-seen wins). When
// all teams exhaust, it emits one document per deduped initiative
// (sorted by ID for determinism), advances to ISSUES, and resets the
// issues cursor. A team page error emits an entity failure and drops
// that team. Returns false if the context was cancelled mid-send.
func (c *LinearConnector) runProjects(
	ctx context.Context,
	cp *LinearCheckpoint,
	initiatives map[string]linearInitiative,
	out chan<- interfaces.DocumentOrFailure,
) bool {
	for len(cp.PendingTeamIDs) > 0 {
		if ctx.Err() != nil {
			return false
		}
		team := cp.PendingTeamIDs[0]

		projects, pi, err := c.client.TeamProjects(ctx, team, cp.ProjectsCursor)
		if err != nil {
			if !interfaces.Send(ctx, out, interfaces.NewDocFailure(
				entityFailure(team, "linear: list team projects", err))) {
				return false
			}
			cp.PendingTeamIDs = cp.PendingTeamIDs[1:] // drop the failing team
			cp.ProjectsCursor = ""
			continue
		}

		for _, p := range projects {
			doc := projectToDocument(p)
			if !interfaces.Send(ctx, out, interfaces.NewDocResult(&doc)) {
				return false
			}
			for _, init := range p.Initiatives.Nodes {
				if _, ok := initiatives[init.ID]; !ok {
					initiatives[init.ID] = init
				}
			}
		}

		if pi.HasNextPage {
			cp.ProjectsCursor = pi.EndCursor
			continue
		}
		cp.PendingTeamIDs = cp.PendingTeamIDs[1:] // team exhausted
		cp.ProjectsCursor = ""
	}

	if !c.emitInitiatives(ctx, initiatives, out) {
		return false
	}
	cp.Stage = StageIssues
	cp.IssuesCursor = ""
	return true
}

// emitInitiatives streams one document per deduped initiative in stable
// (ID-sorted) order. Returns false if the context was cancelled.
func (c *LinearConnector) emitInitiatives(
	ctx context.Context,
	initiatives map[string]linearInitiative,
	out chan<- interfaces.DocumentOrFailure,
) bool {
	ids := make([]string, 0, len(initiatives))
	for id := range initiatives {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		doc := initiativeToDocument(initiatives[id])
		if !interfaces.Send(ctx, out, interfaces.NewDocResult(&doc)) {
			return false
		}
	}
	return true
}

// runIssues pages the combined multi-team issue walk, resolving each
// issue's paginated comments before emitting its document. A page error
// emits an entity failure and aborts the stage (advancing to DONE rather
// than looping). When pages exhaust, the stage advances to DONE.
func (c *LinearConnector) runIssues(
	ctx context.Context,
	cp *LinearCheckpoint,
	updatedSince *time.Time,
	out chan<- interfaces.DocumentOrFailure,
) {
	for {
		if ctx.Err() != nil {
			return
		}

		issues, pi, err := c.client.TeamIssues(ctx, c.cfg.TeamIDs, updatedSince, cp.IssuesCursor)
		if err != nil {
			if !interfaces.Send(ctx, out, interfaces.NewDocFailure(
				entityFailure(issuesEntityID, "linear: list issues", err))) {
				return
			}
			cp.Stage = StageDone // better to end than loop on a hard failure
			return
		}

		for _, issue := range issues {
			comments, ok := c.resolveComments(ctx, issue, out)
			if !ok {
				return
			}
			doc := issueToDocument(issue, comments)
			if !interfaces.Send(ctx, out, interfaces.NewDocResult(&doc)) {
				return
			}
		}

		if pi.HasNextPage {
			cp.IssuesCursor = pi.EndCursor
			continue
		}
		cp.Stage = StageDone
		return
	}
}

// resolveComments returns the issue's fully-resolved comment list: the
// inline first page plus any paginated remainder. On a comment-pagination
// error it emits a per-document failure and falls back to the inline
// page (the doc is still emitted). The bool is false only when the
// context was cancelled mid-send.
func (c *LinearConnector) resolveComments(
	ctx context.Context,
	issue linearIssue,
	out chan<- interfaces.DocumentOrFailure,
) ([]linearComment, bool) {
	comments := issue.Comments.Nodes
	if !issue.Comments.PageInfo.HasNextPage {
		return comments, true
	}

	cursor := issue.Comments.PageInfo.EndCursor
	for {
		if ctx.Err() != nil {
			return nil, false
		}
		page, pi, err := c.client.IssueComments(ctx, issue.ID, cursor)
		if err != nil {
			if !interfaces.Send(ctx, out, interfaces.NewDocFailure(
				interfaces.NewDocumentFailure(
					docIDForIssue(issue.ID), issue.URL,
					"linear: paginate issue comments", err))) {
				return nil, false
			}
			return comments, true // fall back to the inline page
		}
		comments = append(comments, page...)
		if !pi.HasNextPage {
			return comments, true
		}
		cursor = pi.EndCursor
	}
}

// entityFailure builds a non-document failure (a team, or the combined
// issue walk) mirroring github's helper.
func entityFailure(entityID, msg string, cause error) *interfaces.ConnectorFailure {
	return interfaces.NewEntityFailure(entityID, msg, nil, nil, cause)
}
