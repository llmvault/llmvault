package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func TestAgentSessionsImageGenerationToolsE2E(t *testing.T) {
	if os.Getenv("HIVY_AGENT_SESSIONS_E2E") != "1" {
		t.Skip("set HIVY_AGENT_SESSIONS_E2E=1 to run against the live compose stack")
	}
	loadEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	apiBase := agentSessionsBaseURL("HIVY_API_BASE_URL", "HIVY_COMPOSE_API_PORT", "8080")
	workerBase := agentSessionsBaseURL("HIVY_WORKER_BASE_URL", "HIVY_COMPOSE_WORKER_HEALTH_PORT", "8090")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")
	requireAgentSessionsHealthy(t, ctx, workerBase, "worker")
	agentSessionsEnsureSystemOpenRouterCredential(t)

	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	password := "agent-image-generation-e2e-password"
	ownerEmail := "agent-image-generation-" + runID + "@example.com"
	finalMarker := "IMAGE_GENERATION_E2E_PASS_" + runID

	ownerAuth := agentSessionsRegister(t, ctx, apiBase, ownerEmail, password, "Agent Image Generation "+runID)
	orgID := ownerAuth.Orgs[0].ID
	token := ownerAuth.AccessToken

	agent := agentSessionsCreateImageGenerationAgent(t, ctx, apiBase, token, orgID, runID)
	channel := agentSessionsCreateChannel(t, ctx, apiBase, token, orgID, "image-generation-"+runID, agent.ID)
	session := agentSessionsCreateSession(t, ctx, apiBase, token, orgID, channel.ID, strings.Join([]string{
		"This is the flagship image generation tool E2E.",
		"Call hivy_generate_image exactly once with prompt `minimal flat icon of a blue square on white background`, aspect_ratio `1:1`, type `icon`, and count 1.",
		"Then call hivy_generate_vector_image exactly once with description `minimal vector logo of a green circle on transparent background`, aspect_ratio `1:1`, type `logo`, and count 1.",
		"After both tool results, final reply exactly " + finalMarker + " and no other text.",
	}, "\n"))
	if session.Session.ID == "" {
		t.Fatalf("session was not created correctly: %+v", session)
	}

	stream := agentSessionsStartSandboxStream(t, ctx, apiBase, token, orgID, session.Session.ID)
	waitForAgentSessionsImageToolCalls(t, ctx, stream, 4*time.Minute, "hivy_generate_image", "hivy_generate_vector_image")
	imageResults := waitForAgentSessionsImageResult(t, ctx, apiBase, token, orgID, session.Session.ID, "hivy_generate_image", 8*time.Minute)
	assertAgentSessionsGeneratedImageAssets(t, ctx, orgID, agent.ID, "raster", imageResults)

	vectorResults := waitForAgentSessionsImageResult(t, ctx, apiBase, token, orgID, session.Session.ID, "hivy_generate_vector_image", 8*time.Minute)
	assertAgentSessionsGeneratedImageAssets(t, ctx, orgID, agent.ID, "vector", vectorResults)
	artifactDir := filepath.Join(os.TempDir(), "hivy-agent-image-generation-e2e-"+runID)
	imagePaths := saveAgentSessionsImageArtifacts(t, ctx, artifactDir, "raster", imageResults)
	vectorPaths := saveAgentSessionsImageArtifacts(t, ctx, artifactDir, "vector", vectorResults)
	t.Logf("image generation E2E artifacts: %s", strings.Join(append(imagePaths, vectorPaths...), ", "))
	waitForAgentSessionsResponse(t, ctx, apiBase, token, orgID, session.Session.ID, finalMarker)
}

type agentSessionsImageGenerationResult struct {
	DriveAssetID      string   `json:"drive_asset_id"`
	ContentType       string   `json:"content_type"`
	Bytes             int64    `json:"bytes"`
	PublicURL         string   `json:"public_url"`
	ReferenceAssetIDs []string `json:"reference_asset_ids"`
}

func agentSessionsCreateImageGenerationAgent(t *testing.T, ctx context.Context, baseURL, token, orgID, runID string) agentSessionsAgentListItem {
	t.Helper()
	var out agentSessionsAgentMutation
	payload := map[string]any{
		"name":               "Image generation E2E " + runID,
		"instructions":       "Use the image generation tools exactly as requested. Do not use prose instead of tool calls.",
		"model":              agentruntime.DefaultAgentModel,
		"available_models":   []string{agentruntime.DefaultAgentModel},
		"sandbox_strategy":   "per_session",
		"image_model":        registry.DefaultRasterImageGenerationModelID,
		"vector_image_model": registry.DefaultVectorImageGenerationModelID,
	}
	agentSessionsJSON(t, ctx, http.MethodPost, baseURL+"/v1/agents", token, orgID, payload, http.StatusCreated, &out)
	if out.Agent.ID == "" {
		t.Fatalf("image generation agent create returned empty agent: %+v", out)
	}
	return out.Agent
}

func saveAgentSessionsImageArtifacts(t *testing.T, ctx context.Context, dir, mode string, results []agentSessionsImageGenerationResult) []string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create image artifact dir: %v", err)
	}
	client := &http.Client{
		Timeout: 2 * time.Minute,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			agentSessionsRewriteDockerHost(req)
			return nil
		},
	}
	paths := make([]string, 0, len(results))
	for index, result := range results {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, agentSessionsHostAssetURL(result.PublicURL), nil)
		if err != nil {
			t.Fatalf("build %s artifact request: %v", mode, err)
		}
		agentSessionsRewriteDockerHost(req)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("download %s artifact: %v", mode, err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
		closeErr := resp.Body.Close()
		if readErr != nil {
			t.Fatalf("read %s artifact: %v", mode, readErr)
		}
		if closeErr != nil {
			t.Fatalf("close %s artifact: %v", mode, closeErr)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			t.Fatalf("download %s artifact returned %d: %s", mode, resp.StatusCode, strings.TrimSpace(string(body)))
		}
		path := filepath.Join(dir, fmt.Sprintf("%s-%d%s", mode, index+1, agentSessionsImageArtifactExt(result.ContentType)))
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatalf("write %s artifact: %v", mode, err)
		}
		paths = append(paths, path)
	}
	return paths
}

func agentSessionsHostAssetURL(raw string) string {
	raw = strings.Replace(raw, "http://host.docker.internal:", "http://localhost:", 1)
	return strings.Replace(raw, "https://host.docker.internal:", "https://localhost:", 1)
}

func agentSessionsRewriteDockerHost(req *http.Request) {
	if req == nil || !strings.Contains(req.URL.Host, "host.docker.internal") {
		return
	}
	req.Host = req.URL.Host
	req.URL.Host = strings.Replace(req.URL.Host, "host.docker.internal", "localhost", 1)
}

func agentSessionsImageArtifactExt(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/svg+xml":
		return ".svg"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}

func agentSessionsImageGenerationResults(event runtimeSSEEvent) []agentSessionsImageGenerationResult {
	if raw, ok := event.Payload["result"]; ok {
		body, err := json.Marshal(raw)
		if err == nil {
			results := agentSessionsParseImageGenerationResults(body)
			if len(results) > 0 {
				return results
			}
		}
	}
	if summary, _ := event.Payload["result_summary"].(string); summary != "" {
		results := agentSessionsParseImageGenerationResults([]byte(summary))
		if len(results) > 0 {
			return results
		}
	}
	if output := agentSessionsToolResultOutput(event); output != "" {
		return agentSessionsParseImageGenerationResults([]byte(output))
	}
	return nil
}

func agentSessionsParseImageGenerationResults(raw []byte) []agentSessionsImageGenerationResult {
	var results []agentSessionsImageGenerationResult
	if err := json.Unmarshal(raw, &results); err == nil && len(results) > 0 {
		return results
	}
	var envelope struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil
	}
	for _, item := range envelope.Content {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		if results := agentSessionsParseImageGenerationResults([]byte(item.Text)); len(results) > 0 {
			return results
		}
	}
	return nil
}

func waitForAgentSessionsImageToolCalls(t *testing.T, ctx context.Context, stream *agentSessionsLiveSandboxStream, timeout time.Duration, tools ...string) {
	t.Helper()
	want := make(map[string]bool, len(tools))
	for _, tool := range tools {
		want[tool] = true
	}
	seen := make(map[string]bool, len(tools))
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	observed := make([]string, 0, 32)
	for len(seen) < len(want) {
		select {
		case event := <-stream.events:
			tool := eventString(event.Payload, "tool")
			if len(observed) < 80 {
				observed = append(observed, event.Name+":"+tool)
			}
			if event.Name == "tool_call" && want[tool] {
				seen[tool] = true
			}
		case events := <-stream.done:
			t.Fatalf("direct sandbox stream completed before image tool calls; seen=%v events=%s", seen, summarizeRuntimeSSEEvents(events))
		case err := <-stream.errs:
			t.Fatalf("direct sandbox stream failed before image tool calls: %v", err)
		case <-ctx.Done():
			t.Fatalf("direct sandbox stream context ended before image tool calls: %v", ctx.Err())
		case <-timer.C:
			t.Fatalf("timed out waiting for image tool calls; seen=%v observed=%v", seen, observed)
		}
	}
}

func waitForAgentSessionsImageResult(t *testing.T, ctx context.Context, baseURL, token, orgID, sessionID, tool string, timeout time.Duration) []agentSessionsImageGenerationResult {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastEvents []string
	for time.Now().Before(deadline) {
		events := agentSessionsListEvents(t, ctx, baseURL, token, orgID, sessionID)
		lastEvents = lastEvents[:0]
		for _, event := range events {
			lastEvents = append(lastEvents, event.EventType+":"+eventString(event.Payload, "tool"))
			if event.EventType != "tool_result" || eventString(event.Payload, "tool") != tool {
				continue
			}
			results := agentSessionsImageGenerationResults(runtimeSSEEvent{Name: event.EventType, Payload: event.Payload})
			if len(results) > 0 {
				return results
			}
		}
		t.Logf("waiting for image generation result tool=%s events=%v", tool, lastEvents)
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for image generation result tool=%s: %v", tool, ctx.Err())
		case <-time.After(5 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for image generation result tool=%s last_events=%v", tool, lastEvents)
	return nil
}

func assertAgentSessionsGeneratedImageAssets(t *testing.T, ctx context.Context, orgIDRaw, agentIDRaw, mode string, results []agentSessionsImageGenerationResult) {
	t.Helper()
	if len(results) == 0 {
		t.Fatalf("%s generation returned no assets", mode)
	}
	db := agentSessionsOpenDB(t)
	orgID := uuid.MustParse(orgIDRaw)
	agentID := uuid.MustParse(agentIDRaw)
	for _, result := range results {
		if result.DriveAssetID == "" || result.Bytes <= 0 || result.PublicURL == "" || !strings.HasPrefix(result.ContentType, "image/") {
			t.Fatalf("bad %s image generation result: %+v", mode, result)
		}
		assetID := uuid.MustParse(result.DriveAssetID)
		var asset model.AgentAsset
		if err := db.WithContext(ctx).
			Where("id = ? AND org_id = ? AND agent_id = ?", assetID, orgID, agentID).
			First(&asset).Error; err != nil {
			t.Fatalf("load generated %s asset %s: %v", mode, assetID, err)
		}
		if asset.Bytes != result.Bytes || asset.ContentType != result.ContentType {
			t.Fatalf("stored %s asset mismatch row=%+v result=%+v", mode, asset, result)
		}
		if asset.Description == nil {
			t.Fatalf("generated %s asset missing AI metadata: %+v", mode, asset)
		}
		var meta map[string]any
		if err := json.Unmarshal([]byte(*asset.Description), &meta); err != nil {
			t.Fatalf("decode generated %s asset metadata: %v", mode, err)
		}
		if meta["auto_generated"] != true || meta["source"] != "image_generation" || meta["mode"] != mode {
			t.Fatalf("bad generated %s asset metadata: %+v", mode, meta)
		}
	}
}
