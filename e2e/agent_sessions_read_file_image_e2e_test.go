package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
)

func TestAgentSessionsReadFileImageDescribeE2E(t *testing.T) {
	if os.Getenv("HIVY_AGENT_SESSIONS_E2E") != "1" {
		t.Skip("set HIVY_AGENT_SESSIONS_E2E=1 to run against the live compose stack")
	}
	loadEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	apiBase := agentSessionsBaseURL("HIVY_API_BASE_URL", "HIVY_COMPOSE_API_PORT", "8080")
	workerBase := agentSessionsBaseURL("HIVY_WORKER_BASE_URL", "HIVY_COMPOSE_WORKER_HEALTH_PORT", "8090")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")
	requireAgentSessionsHealthy(t, ctx, workerBase, "worker")
	agentSessionsEnsureSystemOpenRouterCredential(t)

	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	password := "agent-read-image-e2e-password"
	ownerEmail := "agent-read-image-" + runID + "@example.com"
	imageText := "HIVY IMAGE READ E2E " + runID
	imageFilename := "read-file-real-image-e2e.png"
	imagePath := "/workspace/" + imageFilename

	ownerAuth := agentSessionsRegister(t, ctx, apiBase, ownerEmail, password, "Agent Read Image "+runID)
	orgID := ownerAuth.Orgs[0].ID
	token := ownerAuth.AccessToken

	agent := agentSessionsCreateReadFileImageAgent(t, ctx, apiBase, token, orgID, runID)
	channel := agentSessionsCreateChannel(t, ctx, apiBase, token, orgID, "read-image-"+runID, agent.ID)
	session := agentSessionsCreateSession(t, ctx, apiBase, token, orgID, channel.ID, "")
	if session.Session.ID == "" || session.Session.SandboxID == nil {
		t.Fatalf("session did not create a per-session sandbox: %+v", session)
	}
	if os.Getenv("HIVY_KEEP_AGENT_SESSIONS_E2E_SANDBOX") != "1" {
		t.Cleanup(func() {
			agentSessionsDeleteSandbox(t, ctx, apiBase, token, orgID, *session.Session.SandboxID)
		})
	}

	sandbox := agentSessionsWaitForSessionSandbox(t, ctx, orgID, session.Session.ID)
	agentSessionsCopyImageFixtureToSandbox(t, ctx, sandbox.ExternalID, imagePath, imageText)
	access := fetchAgentSessionsSandboxAccess(t, ctx, apiBase, token, orgID, session.Session.ID)
	stream := agentSessionsStartSandboxStreamWithAccess(t, ctx, session.Session.ID, access)
	agentSessionsSendMessage(t, ctx, apiBase, token, orgID, session.Session.ID, strings.Join([]string{
		"This is the live read_file image describe E2E.",
		"Call read_file exactly once with path " + imagePath + ".",
		"After the read_file tool result returns, send a short final reply.",
	}, "\n"))
	readCall := stream.waitForEvent(t, ctx, 4*time.Minute, func(event runtimeSSEEvent) bool {
		if event.Name != "tool_call" || eventString(event.Payload, "tool") != "read_file" {
			return false
		}
		raw, _ := json.Marshal(event.Payload["args"])
		return strings.Contains(string(raw), imagePath)
	})
	readCallID := eventString(readCall.Payload, "id")
	if readCallID == "" {
		t.Fatalf("read_file tool_call missing id: %s", readCall.RawData)
	}

	resultEvent := stream.waitForEvent(t, ctx, 4*time.Minute, func(event runtimeSSEEvent) bool {
		return event.Name == "tool_result" && eventString(event.Payload, "id") == readCallID
	})
	assetID := assertAgentSessionsImageReadFileResult(t, resultEvent, imagePath, imageText)
	waitForAgentSessionsImageDescribeGeneration(t, ctx, orgID, agent.ID)
	waitForAgentSessionsDescribedImageAsset(t, ctx, orgID, agent.ID, assetID, imageText)
	terminal := stream.waitForEvent(t, ctx, 2*time.Minute, func(event runtimeSSEEvent) bool {
		return event.Name == "turn_completed" || event.Name == "turn_failed"
	})
	if terminal.Name == "turn_failed" {
		t.Fatalf("turn failed after successful read_file image result: %s", terminal.RawData)
	}
	t.Logf("read_file image turn completed event=%s", terminal.RawData)
}

func agentSessionsCreateReadFileImageAgent(t *testing.T, ctx context.Context, baseURL, token, orgID, runID string) agentSessionsAgentListItem {
	t.Helper()
	var out agentSessionsAgentMutation
	payload := map[string]any{
		"name":             "Read image E2E " + runID,
		"instructions":     "Use the requested tools exactly. Do not describe images from memory; use read_file.",
		"model":            agentruntime.DefaultAgentModel,
		"available_models": []string{agentruntime.DefaultAgentModel},
		"sandbox_strategy": "per_session",
		"tools":            map[string]any{"read_file": true},
	}
	agentSessionsJSON(t, ctx, http.MethodPost, baseURL+"/v1/agents", token, orgID, payload, http.StatusCreated, &out)
	if out.Agent.ID == "" {
		t.Fatalf("read image agent create returned empty agent: %+v", out)
	}
	return out.Agent
}

func agentSessionsCopyImageFixtureToSandbox(t *testing.T, ctx context.Context, externalID, sandboxPath, imageText string) {
	t.Helper()
	localPath := filepath.Join(t.TempDir(), filepath.Base(sandboxPath))
	agentSessionsWriteImageFixture(t, localPath, imageText)
	out, err := exec.CommandContext(ctx, "docker", "cp", localPath, externalID+":"+sandboxPath).CombinedOutput()
	if err != nil {
		t.Fatalf("copy image fixture to sandbox: %v\n%s", err, out)
	}
	out, err = exec.CommandContext(ctx, "docker", "exec", externalID, "sh", "-lc", "test -s "+sandboxPath+" && file "+sandboxPath).CombinedOutput()
	if err != nil {
		t.Fatalf("verify image fixture in sandbox: %v\n%s", err, out)
	}
	t.Logf("sandbox image fixture ready external_id=%s path=%s verify=%s", externalID, sandboxPath, oneLine(string(out)))
}

func agentSessionsWriteImageFixture(t *testing.T, path, imageText string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1200, 620))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	black := color.RGBA{A: 255}
	border := color.RGBA{R: 24, G: 24, B: 24, A: 255}
	for x := 30; x < 1170; x++ {
		for dy := 0; dy < 6; dy++ {
			img.Set(x, 30+dy, border)
			img.Set(x, 584+dy, border)
		}
	}
	for y := 30; y < 590; y++ {
		for dx := 0; dx < 6; dx++ {
			img.Set(30+dx, y, border)
			img.Set(1164+dx, y, border)
		}
	}
	drawScaledBasicText(img, "HIVY IMAGE READ E2E", 92, 145, 7, black)
	drawScaledBasicText(img, imageText, 118, 300, 5, black)
	drawScaledBasicText(img, "LIVE DESCRIBE MODEL SHOULD READ THIS PNG", 96, 430, 4, black)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create image fixture: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode image fixture: %v", err)
	}
}

func drawScaledBasicText(dst *image.RGBA, text string, x, y, scale int, c color.Color) {
	face := basicfont.Face7x13
	width := font.MeasureString(face, text).Ceil()
	height := face.Metrics().Height.Ceil()
	glyph := image.NewRGBA(image.Rect(0, 0, width, height))
	d := font.Drawer{
		Dst:  glyph,
		Src:  &image.Uniform{C: c},
		Face: face,
		Dot:  fixed.P(0, face.Metrics().Ascent.Ceil()),
	}
	d.DrawString(text)
	for gy := 0; gy < height; gy++ {
		for gx := 0; gx < width; gx++ {
			_, _, _, a := glyph.At(gx, gy).RGBA()
			if a == 0 {
				continue
			}
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					dst.Set(x+gx*scale+sx, y+gy*scale+sy, c)
				}
			}
		}
	}
}

func assertAgentSessionsImageReadFileResult(t *testing.T, event runtimeSSEEvent, imagePath, imageText string) uuid.UUID {
	t.Helper()
	result, ok := event.Payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("read_file result is not an object: %s", event.RawData)
	}
	if got := imageReadString(result, "mime_type"); got != "image/png" {
		t.Fatalf("read_file mime_type=%q want image/png result=%s", got, event.RawData)
	}
	if got := imageReadString(result, "content_type"); got != "image/png" {
		t.Fatalf("read_file content_type=%q want image/png result=%s", got, event.RawData)
	}
	if got := imageReadString(result, "path"); got != imagePath && !strings.HasSuffix(got, "/"+filepath.Base(imagePath)) {
		t.Fatalf("read_file path mismatch result=%s", event.RawData)
	}
	assetID, err := uuid.Parse(imageReadString(result, "drive_asset_id"))
	if err != nil {
		t.Fatalf("read_file drive_asset_id invalid: %v result=%s", err, event.RawData)
	}
	content := imageReadString(result, "content")
	description := imageReadString(result, "description")
	analysis, _ := json.Marshal(result["analysis"])
	combined := strings.ToLower(content + "\n" + description + "\n" + string(analysis))
	for _, want := range []string{"hivy", "image", "read", "e2e"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("real image description missing %q from %q result=%s", want, imageText, event.RawData)
		}
	}
	uploaded, ok := result["uploaded_asset"].(map[string]any)
	if !ok || imageReadNumber(uploaded, "bytes") <= 0 {
		t.Fatalf("read_file missing uploaded_asset bytes result=%s", event.RawData)
	}
	t.Logf("live read_file image result asset=%s description=%s", assetID, oneLine(content))
	return assetID
}

func waitForAgentSessionsImageDescribeGeneration(t *testing.T, ctx context.Context, orgIDRaw, agentIDRaw string) model.Generation {
	t.Helper()
	db := agentSessionsOpenDB(t)
	orgID := uuid.MustParse(orgIDRaw)
	deadline := time.Now().Add(3 * time.Minute)
	userID := "runtime:" + agentIDRaw
	var last string
	for time.Now().Before(deadline) {
		var gen model.Generation
		err := db.WithContext(ctx).
			Where("org_id = ? AND user_id = ? AND token_jti = ? AND request_path = ? AND provider_id = ? AND upstream_status = ?",
				orgID, userID, "system:images.describe", "/v1/images/describe", "openrouter", http.StatusOK).
			Order("created_at DESC").
			First(&gen).Error
		if err == nil {
			t.Logf("real image describe generation row id=%s model=%s input_tokens=%d output_tokens=%d", gen.ID, gen.Model, gen.InputTokens, gen.OutputTokens)
			return gen
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			last = err.Error()
		} else {
			last = "not found"
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for image describe generation row: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for image describe generation row org=%s user=%s last=%s", orgID, userID, last)
	return model.Generation{}
}

func waitForAgentSessionsDescribedImageAsset(t *testing.T, ctx context.Context, orgIDRaw, agentIDRaw string, assetID uuid.UUID, imageText string) model.AgentAsset {
	t.Helper()
	db := agentSessionsOpenDB(t)
	orgID := uuid.MustParse(orgIDRaw)
	agentID := uuid.MustParse(agentIDRaw)
	deadline := time.Now().Add(2 * time.Minute)
	var last string
	for time.Now().Before(deadline) {
		var asset model.AgentAsset
		err := db.WithContext(ctx).
			Where("id = ? AND org_id = ? AND agent_id = ? AND content_type = ?", assetID, orgID, agentID, "image/png").
			First(&asset).Error
		if err == nil && asset.Description != nil && len(*asset.Description) > 0 && string(*asset.Description) != "null" {
			body := strings.ToLower(string(*asset.Description))
			for _, want := range []string{"hivy", "image", "read", "e2e"} {
				if !strings.Contains(body, want) {
					t.Fatalf("asset description missing %q from %q: %s", want, imageText, string(*asset.Description))
				}
			}
			t.Logf("real image describe persisted asset=%s bytes=%d filename=%s", asset.ID, asset.Bytes, asset.Filename)
			return asset
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			last = err.Error()
		} else {
			last = "asset not described yet"
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for described image asset: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for described image asset id=%s last=%s", assetID, last)
	return model.AgentAsset{}
}

func imageReadString(m map[string]any, key string) string {
	value, _ := m[key].(string)
	return strings.TrimSpace(value)
}

func imageReadNumber(m map[string]any, key string) float64 {
	switch value := m[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		n, _ := value.Float64()
		return n
	default:
		return 0
	}
}

func oneLine(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 240 {
		return value[:240]
	}
	return value
}
