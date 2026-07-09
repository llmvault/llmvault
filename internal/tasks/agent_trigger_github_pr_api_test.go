package tasks

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/nango"
)

func readGitHubRESTFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "trigger", "dispatch", "testdata", "github", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

// --- parser pinning against canonical REST fixtures ---

func TestParseCheckSuitesFixture(t *testing.T) {
	var resp githubCheckSuitesResponse
	if err := json.Unmarshal(readGitHubRESTFixture(t, "rest_check_suites.list.json"), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TotalCount != 4 || len(resp.CheckSuites) != 4 {
		t.Fatalf("total=%d suites=%d, want 4/4", resp.TotalCount, len(resp.CheckSuites))
	}
	first := resp.CheckSuites[0]
	if first.ID != 5001 || first.Status != "completed" || first.Conclusion != "success" ||
		first.HeadSHA != "d6fde92930d4715a2b49857d24b940956b26d2d3" || first.LatestCheckRunsCount != 3 {
		t.Fatalf("first suite parsed wrong: %+v", first)
	}
	// An in_progress suite is present, so the ref is not settled.
	if checkSuitesSettled(resp.CheckSuites) {
		t.Fatalf("ref with in_progress suite reported settled")
	}
}

func TestCheckSuitesSettledAndSummary(t *testing.T) {
	settled := []githubCheckSuite{
		{ID: 1, Status: "completed", Conclusion: "success", LatestCheckRunsCount: 3},
		{ID: 2, Status: "completed", Conclusion: "failure", LatestCheckRunsCount: 2},
		{ID: 3, Status: "completed", Conclusion: "skipped", LatestCheckRunsCount: 1},
		{ID: 4, Status: "queued", Conclusion: "", LatestCheckRunsCount: 0},
	}
	if !checkSuitesSettled(settled) {
		t.Fatalf("all-runs-complete ref reported unsettled (0-run suite must be ignored)")
	}
	s := summarizeCheckSuites(settled)
	if s.Total != 3 || s.Success != 1 || s.Failure != 1 || s.Neutral != 1 || s.Overall != "failure" {
		t.Fatalf("summary=%+v", s)
	}

	allGreen := []githubCheckSuite{
		{ID: 1, Status: "completed", Conclusion: "success", LatestCheckRunsCount: 1},
		{ID: 2, Status: "completed", Conclusion: "neutral", LatestCheckRunsCount: 1},
	}
	if g := summarizeCheckSuites(allGreen); g.Overall != "success" || g.Failure != 0 {
		t.Fatalf("all-green summary=%+v", g)
	}

	unsettled := []githubCheckSuite{{ID: 1, Status: "in_progress", LatestCheckRunsCount: 1}}
	if checkSuitesSettled(unsettled) {
		t.Fatalf("in_progress ref reported settled")
	}
}

func TestParseReviewCommentsFixture(t *testing.T) {
	var items []githubReviewCommentAPI
	if err := json.Unmarshal(readGitHubRESTFixture(t, "rest_review_comments.list.json"), &items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("comments=%d, want 2", len(items))
	}
	if items[0].Path != "file1.txt" || items[0].Line != 2 || items[0].Body != "Great stuff!" ||
		!strings.HasPrefix(items[0].DiffHunk, "@@ -16,33") {
		t.Fatalf("comment[0]=%+v", items[0])
	}
	// Second comment has line=null; the renderer falls back to original_line.
	if items[1].Line != 0 || reviewCommentLine(items[1]) != 88 {
		t.Fatalf("comment[1] line fallback wrong: line=%d fallback=%d", items[1].Line, reviewCommentLine(items[1]))
	}
}

func TestParseCollaboratorPermissionFixtures(t *testing.T) {
	var write struct {
		Permission string `json:"permission"`
		RoleName   string `json:"role_name"`
	}
	if err := json.Unmarshal(readGitHubRESTFixture(t, "rest_collaborator_permission.write.json"), &write); err != nil {
		t.Fatalf("decode write: %v", err)
	}
	if !githubPermissionAllowsWrite(write.Permission, write.RoleName) {
		t.Fatalf("write fixture (%s/%s) not allowed", write.Permission, write.RoleName)
	}

	var read struct {
		Permission string `json:"permission"`
		RoleName   string `json:"role_name"`
	}
	if err := json.Unmarshal(readGitHubRESTFixture(t, "rest_collaborator_permission.read.json"), &read); err != nil {
		t.Fatalf("decode read: %v", err)
	}
	if githubPermissionAllowsWrite(read.Permission, read.RoleName) {
		t.Fatalf("read fixture (%s/%s) wrongly allowed", read.Permission, read.RoleName)
	}
}

func TestGithubPermissionAllowsWrite(t *testing.T) {
	allow := map[string]bool{
		"admin":    true,
		"maintain": true,
		"write":    true,
		"triage":   false,
		"read":     false,
		"none":     false,
		"":         false,
	}
	for role, want := range allow {
		if got := githubPermissionAllowsWrite("", role); got != want {
			t.Errorf("role_name %q -> %v, want %v", role, got, want)
		}
	}
	// Legacy permission field collapses maintain->write, triage->read.
	if !githubPermissionAllowsWrite("write", "") || !githubPermissionAllowsWrite("admin", "") {
		t.Errorf("legacy write/admin permission not allowed")
	}
	if githubPermissionAllowsWrite("read", "") || githubPermissionAllowsWrite("none", "") {
		t.Errorf("legacy read/none permission wrongly allowed")
	}
}

func TestSuppressReviewBoundComment(t *testing.T) {
	if !suppressReviewBoundComment(prRouteEvent{ReviewID: "42"}) {
		t.Errorf("review-bound comment not suppressed")
	}
	if suppressReviewBoundComment(prRouteEvent{ReviewID: "42", InReplyToID: "10"}) {
		t.Errorf("thread reply wrongly suppressed")
	}
	if suppressReviewBoundComment(prRouteEvent{}) {
		t.Errorf("standalone comment (no review id) wrongly suppressed")
	}
}

// --- programmable GitHub API stub over the Nango proxy ---

type githubAPIStub struct {
	permission   map[string]stubResponse // username -> response
	checkSuites  stubResponse
	reviewCmts   stubResponse
	permCalls    []string
	suiteCalls   int
	commentCalls int
}

type stubResponse struct {
	status int
	body   string
}

func newGitHubAPIStub(t *testing.T) (*nango.Client, *githubAPIStub) {
	t.Helper()
	stub := &githubAPIStub{permission: map[string]stubResponse{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/proxy")
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1,"content":"eyes"}`))
		case strings.HasSuffix(path, "/permission"):
			p := strings.TrimSuffix(path, "/permission")
			user := p[strings.LastIndex(p, "/")+1:]
			stub.permCalls = append(stub.permCalls, user)
			resp := stub.permission[user]
			writeStub(w, resp)
		case strings.HasSuffix(path, "/check-suites"):
			stub.suiteCalls++
			writeStub(w, stub.checkSuites)
		case strings.Contains(path, "/reviews/") && strings.HasSuffix(path, "/comments"):
			stub.commentCalls++
			writeStub(w, stub.reviewCmts)
		default:
			_, _ = io.WriteString(w, "{}")
		}
	}))
	t.Cleanup(srv.Close)
	return nango.NewClient(srv.URL, "secret"), stub
}

func writeStub(w http.ResponseWriter, resp stubResponse) {
	status := resp.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	body := resp.body
	if body == "" {
		body = "{}"
	}
	_, _ = io.WriteString(w, body)
}

// checkSuitesBody builds a settled/unsettled check-suites response body.
func checkSuitesBody(suites ...githubCheckSuite) string {
	resp := githubCheckSuitesResponse{TotalCount: len(suites), CheckSuites: suites}
	b, _ := json.Marshal(resp)
	return string(b)
}
