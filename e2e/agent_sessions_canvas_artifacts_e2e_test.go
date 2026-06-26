package e2e

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type agentSessionsCanvasProjectResponse struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
}

type agentSessionsCanvasArtifactCreateResponse struct {
	ArtifactPath string `json:"artifact_path"`
	ManifestPath string `json:"manifest_path"`
	Slug         string `json:"slug"`
	Type         string `json:"type"`
}

type agentSessionsCanvasArtifactListResponse struct {
	Artifacts []agentSessionsCanvasArtifactResponse `json:"artifacts"`
}

type agentSessionsCanvasArtifactSyncResponse struct {
	Artifact agentSessionsCanvasArtifactResponse `json:"artifact"`
}

type agentSessionsCanvasArtifactResponse struct {
	ID    string `json:"id"`
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Files []struct {
		Path      string `json:"path"`
		SHA256    string `json:"sha256"`
		SizeBytes int64  `json:"size_bytes"`
	} `json:"files"`
}

func TestAgentSessionsCanvasArtifactsCLIE2E(t *testing.T) {
	loadEnv(t)
	if os.Getenv("HIVY_AGENT_SESSIONS_E2E") != "1" {
		t.Skip("set HIVY_AGENT_SESSIONS_E2E=1 to run against the live compose stack")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	apiBase := agentSessionsBaseURL("HIVY_API_BASE_URL", "HIVY_COMPOSE_API_PORT", "8080")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")

	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	password := "agent-sessions-canvas-artifacts-password"
	ownerEmail := "agent-sessions-canvas-artifacts-" + runID + "@example.com"
	ownerAuth := agentSessionsRegister(t, ctx, apiBase, ownerEmail, password, "Canvas Artifacts "+runID)
	orgID := ownerAuth.Orgs[0].ID
	ownerToken := ownerAuth.AccessToken

	agents := agentSessionsListAgents(t, ctx, apiBase, ownerToken, orgID)
	defaultAgent := findDefaultAgent(t, agents)
	assertAgentSessionsAgentSandboxImage(t, "canvas artifacts Hivy", defaultAgent, model.SandboxImageDefault)

	sb := agentSessionsLaunchCanvasRuntime(t, ctx, apiBase, orgID, defaultAgent.ID, runID)
	t.Cleanup(func() {
		agentSessionsDeleteSandbox(t, ctx, apiBase, ownerToken, orgID, sb.ID.String())
	})
	assertAgentSessionsDockerContainer(t, ctx, "canvas artifacts runtime", sb)

	projectName := "Canvas Artifacts " + runID
	project := decodeCanvasProjectResponse(t, agentSessionsDockerCanvas(t, ctx, sb.ExternalID,
		"project", "create", "--name", projectName,
	))
	if project.ID == "" || project.ProjectID == "" || project.Slug == "" || project.Name != projectName {
		t.Fatalf("bad created project: %+v", project)
	}

	created := decodeCanvasArtifactCreateResponse(t, agentSessionsDockerCanvas(t, ctx, sb.ExternalID,
		"artifact", "create",
		"--project", project.Slug,
		"--type", "web_page",
		"--name", "Landing Page "+runID,
	))
	if created.Slug == "" || created.Type != "web_page" {
		t.Fatalf("bad artifact create response: %+v", created)
	}
	if !strings.Contains(created.ArtifactPath, "/workspace/canvas/projects/"+project.Slug+"/artifacts/"+created.Slug) {
		t.Fatalf("artifact path does not use canvas project/artifact layout: %+v", created)
	}

	agentSessionsDockerCanvas(t, ctx, sb.ExternalID, "artifact", "validate", created.ArtifactPath)
	agentSessionsDockerCanvas(t, ctx, sb.ExternalID, "artifact", "verify", created.ManifestPath)

	synced := decodeCanvasArtifactSyncResponse(t, agentSessionsDockerCanvas(t, ctx, sb.ExternalID,
		"artifact", "sync", created.ArtifactPath,
	))
	if synced.Artifact.ID == "" || synced.Artifact.Slug != created.Slug || len(synced.Artifact.Files) != 1 {
		t.Fatalf("bad synced artifact: %+v", synced.Artifact)
	}
	if synced.Artifact.Files[0].Path != "index.html" || synced.Artifact.Files[0].SHA256 == "" {
		t.Fatalf("bad synced artifact file: %+v", synced.Artifact.Files[0])
	}

	listed := decodeCanvasArtifactListResponse(t, agentSessionsDockerCanvas(t, ctx, sb.ExternalID,
		"artifact", "list", "--project", project.ProjectID,
	))
	if !canvasArtifactPageHasSlug(listed, created.Slug) {
		t.Fatalf("artifact list missing slug=%s page=%+v", created.Slug, listed.Artifacts)
	}

	var artifact model.CanvasArtifact
	db := agentSessionsOpenDB(t)
	if err := db.WithContext(ctx).
		Where("org_id = ? AND id = ? AND slug = ? AND archived_at IS NULL", uuid.MustParse(orgID), uuid.MustParse(synced.Artifact.ID), created.Slug).
		First(&artifact).Error; err != nil {
		t.Fatalf("load synced canvas artifact from DB: %v", err)
	}
	var files int64
	if err := db.WithContext(ctx).Model(&model.CanvasArtifactFile{}).
		Where("canvas_artifact_id = ? AND archived_at IS NULL", artifact.ID).
		Count(&files).Error; err != nil {
		t.Fatalf("count synced canvas artifact files: %v", err)
	}
	if files != 1 {
		t.Fatalf("synced file count=%d, want 1", files)
	}
}

func decodeCanvasProjectResponse(t *testing.T, raw []byte) agentSessionsCanvasProjectResponse {
	t.Helper()
	var out agentSessionsCanvasProjectResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode canvas project response: %v\n%s", err, raw)
	}
	return out
}

func decodeCanvasArtifactCreateResponse(t *testing.T, raw []byte) agentSessionsCanvasArtifactCreateResponse {
	t.Helper()
	var out agentSessionsCanvasArtifactCreateResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode canvas artifact create response: %v\n%s", err, raw)
	}
	return out
}

func decodeCanvasArtifactSyncResponse(t *testing.T, raw []byte) agentSessionsCanvasArtifactSyncResponse {
	t.Helper()
	var out agentSessionsCanvasArtifactSyncResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode canvas artifact sync response: %v\n%s", err, raw)
	}
	return out
}

func decodeCanvasArtifactListResponse(t *testing.T, raw []byte) agentSessionsCanvasArtifactListResponse {
	t.Helper()
	var out agentSessionsCanvasArtifactListResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode canvas artifact list response: %v\n%s", err, raw)
	}
	return out
}

func canvasArtifactPageHasSlug(page agentSessionsCanvasArtifactListResponse, slug string) bool {
	for _, artifact := range page.Artifacts {
		if artifact.Slug == slug {
			return true
		}
	}
	return false
}
