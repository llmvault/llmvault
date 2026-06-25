package e2e

import (
	"os"
	"path/filepath"
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
	clonePublicRepo(t, trace, ctx, reposRoot, "https://github.com/octocat/Hello-World.git", "hello-world", 20)
	clonePublicRepo(t, trace, ctx, reposRoot, "https://github.com/sindresorhus/is.git", "is", 20)

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

	tree := fetchRuntimeRepoTree(t, trace, ctx, scenario.baseURL, token, helloRepoID, "")
	assertTreeContains(t, tree, "HIVY_REPO_STREAM_E2E.txt")
	content := fetchRuntimeRepoContent(t, trace, ctx, scenario.baseURL, token, helloRepoID, "HIVY_REPO_STREAM_E2E.txt")
	if !strings.Contains(content.Content, "hello from hivy repo stream e2e") {
		t.Fatalf("repo content missing marker: %+v", content)
	}
	diff := fetchRuntimeRepoDiff(t, trace, ctx, scenario.baseURL, token, helloRepoID, "", 3)
	assertRuntimeDiffFiles(t, diff, []string{"HIVY_REPO_STREAM_E2E.txt"})
	helloDiff := requireRuntimeDiffFile(t, diff, "HIVY_REPO_STREAM_E2E.txt")
	if helloDiff.Status != "untracked" || helloDiff.Truncated {
		t.Fatalf("unexpected hello-world diff file metadata: %+v", helloDiff)
	}
	if !strings.Contains(diff.Diff, "diff --git") || !strings.Contains(diff.Diff, "+hello from hivy repo stream e2e") {
		t.Fatalf("repo diff did not include unified diff marker: %s", diff.Diff)
	}
	if !strings.Contains(helloDiff.Patch, "+hello from hivy repo stream e2e") {
		t.Fatalf("repo file patch missing marker: %+v", helloDiff)
	}

	isBaseSHA := prepareDetachedOlderBaseRepo(t, trace, ctx, filepath.Join(reposRoot, "is"))
	writeRepoFile(t, filepath.Join(reposRoot, "is", "readme.md"), "\nHIVY_REPO_STREAM_E2E_EDIT\n")
	assertRepoChangeEvent(t, stream, ctx, isRepoID, "readme.md")
	updatedRepos := fetchRuntimeRepos(t, trace, ctx, scenario.baseURL, token)
	isRepo := repoByName(t, updatedRepos, "is")
	if isRepo.BaseSHA != isBaseSHA {
		t.Fatalf("base_sha = %s, want checked-out merge base %s", isRepo.BaseSHA, isBaseSHA)
	}
	isDiff := fetchRuntimeRepoDiff(t, trace, ctx, scenario.baseURL, token, isRepoID, "", 3)
	assertRuntimeDiffFiles(t, isDiff, []string{"readme.md"})
	readmeDiff := requireRuntimeDiffFile(t, isDiff, "readme.md")
	if readmeDiff.Status != "modified" || readmeDiff.Truncated {
		t.Fatalf("unexpected is/readme.md diff file metadata: %+v", readmeDiff)
	}
	if !strings.Contains(readmeDiff.Patch, "+HIVY_REPO_STREAM_E2E_EDIT") {
		t.Fatalf("older-base diff missing local edit: %s", readmeDiff.Patch)
	}
	if strings.Contains(isDiff.Diff, "HIVY_REMOTE_ONLY") || strings.Contains(isDiff.Diff, "HIVY_REMOTE_ONLY.txt") {
		t.Fatalf("older-base diff included remote-tip-only changes: %s", isDiff.Diff)
	}

	clonePublicRepo(t, trace, ctx, reposRoot, "https://github.com/tj/commander.js.git", "commander-js", 1)
	waitForRuntimeRepo(t, trace, ctx, scenario.baseURL, token, "commander-js")
	writeRepoFile(t, filepath.Join(reposRoot, "commander-js", "HIVY_LATE_REPO.txt"), "late repo edit\n")
	lateRepos := fetchRuntimeRepos(t, trace, ctx, scenario.baseURL, token)
	lateRepoID := repoIDByName(t, lateRepos, "commander-js")
	assertRepoChangeEvent(t, stream, ctx, lateRepoID, "HIVY_LATE_REPO.txt")
}
