package linear

// issueFields is the shared selection set for a full issue: scalars plus
// team/state/assignee/creator/project and the first page of labels (25)
// and comments (25).
const issueFields = `id createdAt updatedAt identifier title description url priorityLabel dueDate
        team { id name key }
        state { name }
        assignee { name email }
        creator { name email }
        project { id name }
        labels(first: 25) { nodes { name } }
        comments(first: 25) {
          nodes { id body url updatedAt user { name email } }
          pageInfo { hasNextPage endCursor }
        }`

// teamFilter matches issues in any of the requested teams.
const teamFilter = `{ team: { id: { in: $teamIds } } }`

// incrementalFilter matches issues in the requested teams that were
// themselves updated since $since OR have any comment updated since
// $since — so a fresh comment on an otherwise-stale issue is not missed.
const incrementalFilter = `{ and: [ { team: { id: { in: $teamIds } } }, { or: [ { updatedAt: { gte: $since } }, { comments: { some: { updatedAt: { gte: $since } } } } ] } ] }`

// buildTeamIssuesQuery returns the TeamIssues query. When incremental it
// declares $since (DateTimeOrDuration) and uses the comment-robust
// filter; otherwise it selects every non-archived issue in the teams.
// Both order by updatedAt so pagination is stable.
func buildTeamIssuesQuery(incremental bool) string {
	if incremental {
		return `query TeamIssues($teamIds: [ID!], $since: DateTimeOrDuration, $cursor: String) {
  issues(first: 50, after: $cursor, orderBy: updatedAt, filter: ` + incrementalFilter + `) {
    nodes {
        ` + issueFields + `
    }
    pageInfo { hasNextPage endCursor }
  }
}`
	}
	return `query TeamIssues($teamIds: [ID!], $cursor: String) {
  issues(first: 50, after: $cursor, orderBy: updatedAt, filter: ` + teamFilter + `) {
    nodes {
        ` + issueFields + `
    }
    pageInfo { hasNextPage endCursor }
  }
}`
}
