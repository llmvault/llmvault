package linear

import "encoding/json"

// graphQLBody marshals a GraphQL request into the {query, variables,
// operationName} shape Linear expects. operationName is ALWAYS included:
// it is how the test fake (and Linear itself) dispatches a request.
// variables is omitted entirely when empty.
func graphQLBody(opName, query string, vars map[string]any) ([]byte, error) {
	payload := map[string]any{
		"operationName": opName,
		"query":         query,
	}
	if len(vars) > 0 {
		payload["variables"] = vars
	}
	return json.Marshal(payload)
}

// teamsQuery lists every team, 100 per page. Archived teams are excluded
// by default (no includeArchived).
const teamsQuery = `query Teams($cursor: String) {
  teams(first: 100, after: $cursor) {
    nodes { id name key }
    pageInfo { hasNextPage endCursor }
  }
}`

// teamProjectsQuery lists a single team's projects (50 per page) with the
// first 50 initiatives nested on each project.
const teamProjectsQuery = `query TeamProjects($teamId: String!, $cursor: String) {
  team(id: $teamId) {
    projects(first: 50, after: $cursor) {
      nodes {
        id name description content url updatedAt
        initiatives(first: 50) {
          nodes { id name description content url updatedAt }
        }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}`

// issueCommentsQuery pages the comments of a single issue, 50 per page.
const issueCommentsQuery = `query IssueComments($id: String!, $cursor: String) {
  issue(id: $id) {
    comments(first: 50, after: $cursor) {
      nodes { id body url updatedAt user { name email } }
      pageInfo { hasNextPage endCursor }
    }
  }
}`

// slimTeamIssuesQuery lists issue ids only across the given teams, 100 per
// page. No updatedAt filter — the pruning loop needs the full current set.
const slimTeamIssuesQuery = `query SlimTeamIssues($teamIds: [ID!], $cursor: String) {
  issues(first: 100, after: $cursor, filter: { team: { id: { in: $teamIds } } }) {
    nodes { id }
    pageInfo { hasNextPage endCursor }
  }
}`
