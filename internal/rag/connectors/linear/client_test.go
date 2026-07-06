package linear

import (
	"context"
	"strings"
	"testing"
	"time"
)

// newTestClient wires a client to a fake proxy with a no-op sleeper so
// retry paths don't actually block; recorded sleeps land in *sleeps.
func newTestClient(fp *fakeProxy, sleeps *[]time.Duration) *Client {
	c := newClient(fp)
	c.sleep = func(_ context.Context, d time.Duration) error {
		*sleeps = append(*sleeps, d)
		return nil
	}
	return c
}

func lastBody(t *testing.T, fp *fakeProxy) string {
	t.Helper()
	bodies := fp.requestBodies()
	if len(bodies) == 0 {
		t.Fatal("no requests recorded")
	}
	return bodies[len(bodies)-1]
}

func TestTeamsPaginates(t *testing.T) {
	fp := newFakeProxy()
	fp.stub("Teams", 200, `{"data":{"teams":{"nodes":[{"id":"t1","name":"One","key":"ONE"}],"pageInfo":{"hasNextPage":true,"endCursor":"c1"}}}}`)
	fp.stub("Teams", 200, `{"data":{"teams":{"nodes":[{"id":"t2","name":"Two","key":"TWO"}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}`)

	var sleeps []time.Duration
	c := newTestClient(fp, &sleeps)
	teams, err := c.Teams(context.Background())
	if err != nil {
		t.Fatalf("Teams: %v", err)
	}
	if len(teams) != 2 || teams[0].ID != "t1" || teams[1].Key != "TWO" {
		t.Fatalf("unexpected teams: %+v", teams)
	}
	bodies := fp.requestBodies()
	if len(bodies) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(bodies))
	}
	if !strings.Contains(bodies[1], `"cursor":"c1"`) {
		t.Errorf("second request should carry cursor c1: %s", bodies[1])
	}
	if !strings.Contains(bodies[0], `"operationName":"Teams"`) {
		t.Errorf("request missing operationName: %s", bodies[0])
	}
}

func TestTeamIssuesIncrementalFilter(t *testing.T) {
	fp := newFakeProxy()
	fp.stub("TeamIssues", 200, `{"data":{"issues":{"nodes":[{"id":"i1","identifier":"ENG-1","title":"Bug"}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}`)

	var sleeps []time.Duration
	c := newTestClient(fp, &sleeps)
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	issues, _, err := c.TeamIssues(context.Background(), []string{"t1", "t2"}, &since, "")
	if err != nil {
		t.Fatalf("TeamIssues: %v", err)
	}
	if len(issues) != 1 || issues[0].Identifier != "ENG-1" {
		t.Fatalf("unexpected issues: %+v", issues)
	}
	body := lastBody(t, fp)
	for _, want := range []string{`"operationName":"TeamIssues"`, `and`, `or`, `gte`, `$since`, `2026-07-01T00:00:00Z`, `"t1"`, `"t2"`} {
		if !strings.Contains(body, want) {
			t.Errorf("incremental request missing %q in body: %s", want, body)
		}
	}
}

func TestTeamIssuesFullFilterOmitsSince(t *testing.T) {
	fp := newFakeProxy()
	fp.stub("TeamIssues", 200, `{"data":{"issues":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}`)

	var sleeps []time.Duration
	c := newTestClient(fp, &sleeps)
	if _, _, err := c.TeamIssues(context.Background(), []string{"t1"}, nil, ""); err != nil {
		t.Fatalf("TeamIssues: %v", err)
	}
	body := lastBody(t, fp)
	if strings.Contains(body, "$since") || strings.Contains(body, "gte") {
		t.Errorf("full sync should not reference $since/gte: %s", body)
	}
	if !strings.Contains(body, `team`) || !strings.Contains(body, `in`) {
		t.Errorf("full sync should carry team-in filter: %s", body)
	}
}

func TestTeamProjectsNestedInitiatives(t *testing.T) {
	fp := newFakeProxy()
	fp.stub("TeamProjects", 200, `{"data":{"team":{"projects":{"nodes":[{"id":"p1","name":"Proj","initiatives":{"nodes":[{"id":"in1","name":"Init"}]}}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`)

	var sleeps []time.Duration
	c := newTestClient(fp, &sleeps)
	projects, pi, err := c.TeamProjects(context.Background(), "t1", "")
	if err != nil {
		t.Fatalf("TeamProjects: %v", err)
	}
	if len(projects) != 1 || projects[0].ID != "p1" {
		t.Fatalf("unexpected projects: %+v", projects)
	}
	if len(projects[0].Initiatives.Nodes) != 1 || projects[0].Initiatives.Nodes[0].Name != "Init" {
		t.Fatalf("nested initiative not parsed: %+v", projects[0].Initiatives)
	}
	if pi.HasNextPage {
		t.Errorf("expected no next page")
	}
}

func TestIssueCommentsPaginates(t *testing.T) {
	fp := newFakeProxy()
	fp.stub("IssueComments", 200, `{"data":{"issue":{"comments":{"nodes":[{"id":"c1","body":"hi"}],"pageInfo":{"hasNextPage":true,"endCursor":"cc"}}}}}`)

	var sleeps []time.Duration
	c := newTestClient(fp, &sleeps)
	comments, pi, err := c.IssueComments(context.Background(), "i1", "")
	if err != nil {
		t.Fatalf("IssueComments: %v", err)
	}
	if len(comments) != 1 || comments[0].ID != "c1" {
		t.Fatalf("unexpected comments: %+v", comments)
	}
	if !pi.HasNextPage || pi.EndCursor != "cc" {
		t.Errorf("expected next page cc, got %+v", pi)
	}
}

func TestSlimTeamIssuesIDsOnly(t *testing.T) {
	fp := newFakeProxy()
	fp.stub("SlimTeamIssues", 200, `{"data":{"issues":{"nodes":[{"id":"i1"},{"id":"i2"}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}`)

	var sleeps []time.Duration
	c := newTestClient(fp, &sleeps)
	ids, _, err := c.SlimTeamIssues(context.Background(), []string{"t1"}, "")
	if err != nil {
		t.Fatalf("SlimTeamIssues: %v", err)
	}
	if len(ids) != 2 || ids[0].ID != "i1" || ids[1].ID != "i2" {
		t.Fatalf("unexpected slim ids: %+v", ids)
	}
	body := lastBody(t, fp)
	if strings.Contains(body, "updatedAt") {
		t.Errorf("slim query must not filter on updatedAt: %s", body)
	}
}

func TestGraphQLErrorsSurfaced(t *testing.T) {
	fp := newFakeProxy()
	fp.stub("Teams", 200, `{"errors":[{"message":"Something broke"},{"message":"and again"}]}`)

	var sleeps []time.Duration
	c := newTestClient(fp, &sleeps)
	_, err := c.Teams(context.Background())
	if err == nil {
		t.Fatal("expected error from GraphQL errors")
	}
	if !strings.Contains(err.Error(), "Something broke") || !strings.Contains(err.Error(), "and again") {
		t.Errorf("error should surface GraphQL messages: %v", err)
	}
	// GraphQL errors are permanent: exactly one request, no retries.
	if got := len(fp.requestBodies()); got != 1 {
		t.Errorf("expected 1 request (no retry), got %d", got)
	}
	if len(sleeps) != 0 {
		t.Errorf("expected no sleeps on permanent error, got %v", sleeps)
	}
}
