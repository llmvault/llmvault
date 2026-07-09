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

	"github.com/usehivy/hivy/internal/model"
)

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
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, agentSessionsHostReachableURL(result.PublicURL), nil)
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
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatalf("write %s artifact: %v", mode, err)
		}
		paths = append(paths, path)
	}
	return paths
}

func agentSessionsHostReachableURL(raw string) string {
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

func assertAgentSessionsGeneratedImageAssets(t *testing.T, ctx context.Context, orgIDRaw, agentIDRaw, mode, providerID, modelID string, results []agentSessionsImageGenerationResult) {
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
		if meta["provider_id"] != providerID || meta["model"] != modelID {
			t.Fatalf("bad generated %s model metadata: %+v", mode, meta)
		}
	}
}
