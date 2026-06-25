package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func clonePublicRepo(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, reposRoot, remote, name string, depth int) {
	t.Helper()
	target := filepath.Join(reposRoot, name)
	if _, err := os.Stat(target); err == nil {
		return
	}
	trace.Logf("repo-stream", "cloning %s into %s", remote, target)
	runCommand(t, trace, ctx, reposRoot, "git", "clone", "--depth", strconv.Itoa(depth), remote, name)
}

func prepareDetachedOlderBaseRepo(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, repoPath string) string {
	t.Helper()
	runCommand(t, trace, ctx, repoPath, "git", "config", "user.email", "repo-stream-e2e@example.com")
	runCommand(t, trace, ctx, repoPath, "git", "config", "user.name", "Repo Stream E2E")
	baseSHA := strings.TrimSpace(string(runCommand(t, trace, ctx, repoPath, "git", "rev-parse", "HEAD")))
	remoteHead := strings.TrimSpace(string(runCommand(t, trace, ctx, repoPath, "git", "symbolic-ref", "--short", "refs/remotes/origin/HEAD")))
	if remoteHead == "" {
		t.Fatalf("origin/HEAD is not set in %s", repoPath)
	}
	remoteRef := "refs/remotes/" + remoteHead
	runCommand(t, trace, ctx, repoPath, "git", "checkout", "-b", "hivy-remote-tip")
	writeRepoFile(t, filepath.Join(repoPath, "HIVY_REMOTE_ONLY.txt"), "remote tip only\n")
	runCommand(t, trace, ctx, repoPath, "git", "add", "HIVY_REMOTE_ONLY.txt")
	runCommand(t, trace, ctx, repoPath, "git", "commit", "-m", "remote tip only")
	remoteTip := strings.TrimSpace(string(runCommand(t, trace, ctx, repoPath, "git", "rev-parse", "HEAD")))
	runCommand(t, trace, ctx, repoPath, "git", "update-ref", remoteRef, remoteTip)
	runCommand(t, trace, ctx, repoPath, "git", "checkout", "--detach", baseSHA)
	return baseSHA
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
	RepoID     string                `json:"repo_id"`
	Path       *string               `json:"path"`
	Diff       string                `json:"diff"`
	Truncated  bool                  `json:"truncated"`
	TotalBytes int                   `json:"total_bytes"`
	MaxBytes   int                   `json:"max_bytes"`
	Files      []runtimeRepoDiffFile `json:"files"`
	Message    *string               `json:"message"`
}

type runtimeRepoDiffFile struct {
	Path         string  `json:"path"`
	Status       string  `json:"status"`
	PreviousPath *string `json:"previous_path"`
	Patch        string  `json:"patch"`
	Truncated    bool    `json:"truncated"`
	TotalBytes   int     `json:"total_bytes"`
	MaxBytes     int     `json:"max_bytes"`
	Message      *string `json:"message"`
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
	query := url.Values{}
	if path != "" {
		query.Set("path", path)
	}
	if contextLines > 0 {
		query.Set("context", strconv.Itoa(contextLines))
	}
	endpoint := baseURL + "/repos/" + url.PathEscape(repoID) + "/diff"
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	doRuntimeRepoJSON(t, trace, ctx, http.MethodGet, endpoint, token, &out)
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
	return repoByName(t, repos, name).ID
}

func repoByName(t *testing.T, repos runtimeReposResponse, name string) runtimeRepoInfo {
	t.Helper()
	for _, repo := range repos.Repos {
		if repo.Name == name {
			return repo
		}
	}
	t.Fatalf("repo %s not found in %+v", name, repos.Repos)
	return runtimeRepoInfo{}
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

func assertRuntimeDiffFiles(t *testing.T, diff runtimeRepoDiffResponse, want []string) {
	t.Helper()
	got := make([]string, 0, len(diff.Files))
	for _, file := range diff.Files {
		got = append(got, file.Path)
	}
	sort.Strings(got)
	want = append([]string(nil), want...)
	sort.Strings(want)
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("diff files = %v, want %v; diff=%s", got, want, diff.Diff)
	}
}

func requireRuntimeDiffFile(t *testing.T, diff runtimeRepoDiffResponse, path string) runtimeRepoDiffFile {
	t.Helper()
	for _, file := range diff.Files {
		if file.Path == path {
			return file
		}
	}
	t.Fatalf("diff missing file %s: %+v", path, diff.Files)
	return runtimeRepoDiffFile{}
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
