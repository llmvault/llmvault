package linear

import (
	"context"
	"sort"

	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// ListAllSlim streams every current document ID, reusing the same
// listing endpoints as ingest so the prune diff sees the exact same
// doc-id alphabet: docIDForProject, docIDForInitiative (deduped across
// teams), and docIDForIssue. The walk is identical to ingest except it
// applies no updatedSince filter.
func (c *LinearConnector) ListAllSlim(
	ctx context.Context, _ interfaces.Source,
) (<-chan interfaces.SlimDocOrFailure, error) {
	out := make(chan interfaces.SlimDocOrFailure, c.channelBuf)
	goroutine.Go(ctx, func(ctx context.Context) {
		defer close(out)
		c.streamSlim(ctx, out)
	})
	return out, nil
}

// streamSlim performs the projects → initiatives → issues walk. It
// returns early (silently) when the context is cancelled — the caller's
// deferred close(out) unwinds the producer.
func (c *LinearConnector) streamSlim(
	ctx context.Context,
	out chan<- interfaces.SlimDocOrFailure,
) {
	initiatives := make(map[string]struct{})

	for _, team := range c.cfg.TeamIDs {
		cursor := ""
		for {
			if ctx.Err() != nil {
				return
			}
			projects, pi, err := c.client.TeamProjects(ctx, team, cursor)
			if err != nil {
				if !interfaces.Send(ctx, out, interfaces.NewSlimFailure(
					entityFailure(team, "linear: list team projects slim", err))) {
					return
				}
				break // drop the failing team, continue with the rest
			}
			for _, p := range projects {
				if !interfaces.Send(ctx, out, interfaces.NewSlimResult(
					&interfaces.SlimDocument{DocID: docIDForProject(p.ID)})) {
					return
				}
				for _, init := range p.Initiatives.Nodes {
					initiatives[init.ID] = struct{}{}
				}
			}
			if !pi.HasNextPage {
				break
			}
			cursor = pi.EndCursor
		}
	}

	if !c.emitInitiativeSlims(ctx, initiatives, out) {
		return
	}
	c.streamIssueSlims(ctx, out)
}

// emitInitiativeSlims streams the deduped initiative IDs in stable
// (ID-sorted) order. Returns false if the context was cancelled.
func (c *LinearConnector) emitInitiativeSlims(
	ctx context.Context,
	initiatives map[string]struct{},
	out chan<- interfaces.SlimDocOrFailure,
) bool {
	ids := make([]string, 0, len(initiatives))
	for id := range initiatives {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if !interfaces.Send(ctx, out, interfaces.NewSlimResult(
			&interfaces.SlimDocument{DocID: docIDForInitiative(id)})) {
			return false
		}
	}
	return true
}

// streamIssueSlims pages the combined multi-team issue walk, emitting a
// slim doc per issue.
func (c *LinearConnector) streamIssueSlims(
	ctx context.Context,
	out chan<- interfaces.SlimDocOrFailure,
) {
	cursor := ""
	for {
		if ctx.Err() != nil {
			return
		}
		issues, pi, err := c.client.SlimTeamIssues(ctx, c.cfg.TeamIDs, cursor)
		if err != nil {
			if !interfaces.Send(ctx, out, interfaces.NewSlimFailure(
				entityFailure(issuesEntityID, "linear: list issues slim", err))) {
				return
			}
			return
		}
		for _, iss := range issues {
			if !interfaces.Send(ctx, out, interfaces.NewSlimResult(
				&interfaces.SlimDocument{DocID: docIDForIssue(iss.ID)})) {
				return
			}
		}
		if !pi.HasNextPage {
			return
		}
		cursor = pi.EndCursor
	}
}
