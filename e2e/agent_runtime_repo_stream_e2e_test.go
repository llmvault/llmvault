package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAgentRuntimeRepositoryStreamingE2E(t *testing.T) {
	ctx, cancel, trace := agentRuntimeE2EContext(t, 10*time.Minute)
	defer cancel()

	workspaceRoot := t.TempDir()
	reposRoot := filepath.Join(workspaceRoot, "repos")
	if err := os.MkdirAll(reposRoot, 0755); err != nil {
		t.Fatalf("create repos root: %v", err)
	}
	clonePublicRepo(t, trace, ctx, reposRoot, "https://github.com/octocat/Hello-World.git", "hello-world")
	clonePublicRepo(t, trace, ctx, reposRoot, "https://github.com/sindresorhus/is.git", "is")

	scenario := startAgentRuntimeE2EScenario(
		t,
		trace,
		ctx,
		agentRuntimeE2EScenarioOptions{name: "repo-stream", workspaceRoot: workspaceRoot},
		func(proxyURL, controlPlaneURL, sandboxID string) map[string]any {
			return agentRuntimeUpdatePlanDefinition(t, trace, proxyURL, controlPlaneURL, sandboxID)
		},
	)

	sessionID := "agent-runtime-repo-stream-e2e"
	token := directRuntimeJWT(t, scenario.runtimeSecret, sessionID, scenario.sandboxID, "stream:read", "repo:read")
	stream := startDirectRuntimeLiveStream(
		t,
		trace,
		ctx,
		directRuntimeStreamURL(t, scenario.baseURL, "/sessions/"+sessionID+"/stream?replay=none"),
		token,
	)

	repos := fetchRuntimeRepos(t, trace, ctx, scenario.baseURL, token)
	helloRepoID := repoIDByName(t, repos, "hello-world")
	isRepoID := repoIDByName(t, repos, "is")

	writeRepoFile(t, filepath.Join(reposRoot, "hello-world", "HIVY_REPO_STREAM_E2E.txt"), "hello from hivy repo stream e2e\n")
	assertRepoChangeEvent(t, stream, ctx, helloRepoID, "HIVY_REPO_STREAM_E2E.txt")

	writeRepoFile(t, filepath.Join(reposRoot, "is", "readme.md"), "\nHIVY_REPO_STREAM_E2E_EDIT\n")
	assertRepoChangeEvent(t, stream, ctx, isRepoID, "readme.md")

	tree := fetchRuntimeRepoTree(t, trace, ctx, scenario.baseURL, token, helloRepoID, "")
	assertTreeContains(t, tree, "HIVY_REPO_STREAM_E2E.txt")
	content := fetchRuntimeRepoContent(t, trace, ctx, scenario.baseURL, token, helloRepoID, "HIVY_REPO_STREAM_E2E.txt")
	if !strings.Contains(content.Content, "hello from hivy repo stream e2e") {
		t.Fatalf("repo content missing marker: %+v", content)
	}
	diff := fetchRuntimeRepoDiff(t, trace, ctx, scenario.baseURL, token, helloRepoID, "HIVY_REPO_STREAM_E2E.txt", 3)
	if !strings.Contains(diff.Diff, "diff --git") || !strings.Contains(diff.Diff, "+hello from hivy repo stream e2e") {
		t.Fatalf("repo diff did not include unified diff marker: %s", diff.Diff)
	}

	clonePublicRepo(t, trace, ctx, reposRoot, "https://github.com/tj/commander.js.git", "commander-js")
	waitForRuntimeRepo(t, trace, ctx, scenario.baseURL, token, "commander-js")
	writeRepoFile(t, filepath.Join(reposRoot, "commander-js", "HIVY_LATE_REPO.txt"), "late repo edit\n")
	lateRepos := fetchRuntimeRepos(t, trace, ctx, scenario.baseURL, token)
	lateRepoID := repoIDByName(t, lateRepos, "commander-js")
	assertRepoChangeEvent(t, stream, ctx, lateRepoID, "HIVY_LATE_REPO.txt")
}

func clonePublicRepo(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, reposRoot, remote, name string) {
	t.Helper()
	target := filepath.Join(reposRoot, name)
	if _, err := os.Stat(target); err == nil {
		return
	}
	trace.Logf("repo-stream", "cloning %s into %s", remote, target)
	runCommand(t, trace, ctx, reposRoot, "git", "clone", "--depth", "1", remote, name)
}

func writeRepoFile(t *testing.T, path, content string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatalf("open repo file %s: %v", path, err)
	}
	defer file.Close()
	if _, err := file.WriteString(content); err != nil {
		t.Fatalf("write repo file %s: %v", path, err)
	}
}

type runtimeReposResponse struct {
	Repos []runtimeRepoInfo `json:"repos"`
}

type runtimeRepoInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	RelativePath string `json:"relative_path"`
	HeadSHA      string `json:"head_sha"`
	BaseSHA      string `json:"base_sha"`
}

type runtimeRepoTreeResponse struct {
	RepoID  string                 `json:"repo_id"`
	Path    string                 `json:"path"`
	Entries []runtimeRepoTreeEntry `json:"entries"`
}

type runtimeRepoTreeEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Type      string `json:"type"`
	GitStatus string `json:"git_status"`
}

type runtimeRepoContentResponse struct {
	RepoID    string `json:"repo_id"`
	Path      string `json:"path"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

type runtimeRepoDiffResponse struct {
	RepoID string `json:"repo_id"`
	Path   string `json:"path"`
	Diff   string `json:"diff"`
}

func fetchRuntimeRepos(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, baseURL, token string) runtimeReposResponse {
	t.Helper()
	var out runtimeReposResponse
	doRuntimeRepoJSON(t, trace, ctx, http.MethodGet, baseURL+"/repos", token, &out)
	return out
}

func fetchRuntimeRepoTree(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, baseURL, token, repoID, path string) runtimeRepoTreeResponse {
	t.Helper()
	var out runtimeRepoTreeResponse
	doRuntimeRepoJSON(t, trace, ctx, http.MethodGet, baseURL+"/repos/"+url.PathEscape(repoID)+"/tree?path="+url.QueryEscape(path), token, &out)
	return out
}

func fetchRuntimeRepoContent(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, baseURL, token, repoID, path string) runtimeRepoContentResponse {
	t.Helper()
	var out runtimeRepoContentResponse
	doRuntimeRepoJSON(t, trace, ctx, http.MethodGet, baseURL+"/repos/"+url.PathEscape(repoID)+"/content?path="+url.QueryEscape(path), token, &out)
	return out
}

func fetchRuntimeRepoDiff(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, baseURL, token, repoID, path string, contextLines int) runtimeRepoDiffResponse {
	t.Helper()
	var out runtimeRepoDiffResponse
	doRuntimeRepoJSON(t, trace, ctx, http.MethodGet, baseURL+"/repos/"+url.PathEscape(repoID)+"/diff?path="+url.QueryEscape(path)+"&context="+url.QueryEscape(strconv.Itoa(contextLines)), token, &out)
	return out
}

func doRuntimeRepoJSON(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, method, endpoint, token string, out any) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		t.Fatalf("new repo API request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("repo API request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("repo API status=%d endpoint=%s", resp.StatusCode, endpoint)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode repo API response from %s: %v", endpoint, err)
	}
	trace.Logf("repo-stream", "%s %s ok", method, endpoint)
}

func repoIDByName(t *testing.T, repos runtimeReposResponse, name string) string {
	t.Helper()
	for _, repo := range repos.Repos {
		if repo.Name == name {
			return repo.ID
		}
	}
	t.Fatalf("repo %s not found in %+v", name, repos.Repos)
	return ""
}

func waitForRuntimeRepo(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, baseURL, token, name string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		repos := fetchRuntimeRepos(t, trace, ctx, baseURL, token)
		for _, repo := range repos.Repos {
			if repo.Name == name {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for repo %s", name)
}

func assertTreeContains(t *testing.T, tree runtimeRepoTreeResponse, path string) {
	t.Helper()
	for _, entry := range tree.Entries {
		if entry.Path == path {
			return
		}
	}
	t.Fatalf("tree missing %s entries=%+v", path, tree.Entries)
}

func assertRepoChangeEvent(t *testing.T, stream *directRuntimeLiveStream, ctx context.Context, repoID, path string) {
	t.Helper()
	event := stream.waitForEvent(t, ctx, 30*time.Second, func(event runtimeSSEEvent) bool {
		if event.Name != "repo.change_batch" || event.Payload["repo_id"] != repoID {
			return false
		}
		paths, ok := event.Payload["paths"].([]any)
		if !ok {
			return false
		}
		for _, item := range paths {
			changed, ok := item.(map[string]any)
			if ok && changed["path"] == path {
				return true
			}
		}
		return false
	})
	if event.Payload["sequence"] == nil || event.Payload["summary"] == nil {
		t.Fatalf("repo change event missing sequence or summary: %+v", event.Payload)
	}
}
