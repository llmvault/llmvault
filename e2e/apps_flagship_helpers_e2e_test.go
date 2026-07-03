package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/storage"
)

// appsFlagshipResponse is a fully-read HTTP exchange result for the direct
// app/API probes the flagship test makes (no helper JSON decoding, because
// most assertions are on status codes, redirect Locations, and cookies).
type appsFlagshipResponse struct {
	Status int
	Header http.Header
	Body   []byte
}

// appsFlagshipDo issues one request WITHOUT following redirects — the launch
// handoff assertions depend on observing each 302 hop and its cookies.
func appsFlagshipDo(t *testing.T, ctx context.Context, method, rawURL string, headers map[string]string, body []byte) appsFlagshipResponse {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, rawURL, err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       60 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, rawURL, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s %s body: %v", method, rawURL, err)
	}
	return appsFlagshipResponse{Status: resp.StatusCode, Header: resp.Header, Body: raw}
}

// appsFlagshipRewriteHost maps a URL the API resolved for ITS vantage point
// (inside compose, sandbox ports are reached via host.docker.internal) onto
// the host's vantage point (the docker port bindings are on 127.0.0.1). Only
// the hostname changes; scheme, port, path, and query are untouched.
func appsFlagshipRewriteHost(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL %q: %v", raw, err)
	}
	if u.Hostname() == "host.docker.internal" {
		port := u.Port()
		u.Host = "127.0.0.1"
		if port != "" {
			u.Host = "127.0.0.1:" + port
		}
	}
	return u.String()
}

// appsFlagshipContainerHostPort resolves the published host port for a
// container port — the same NetworkSettings.Ports lookup the docker
// provider's GetEndpoint performs — so the test reaches the app exactly the
// way the platform does.
func appsFlagshipContainerHostPort(t *testing.T, ctx context.Context, externalID string, containerPort int) string {
	t.Helper()
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("create docker client: %v", err)
	}
	defer cli.Close()
	info, err := cli.ContainerInspect(ctx, externalID)
	if err != nil {
		t.Fatalf("inspect app sandbox container %s: %v", externalID, err)
	}
	if info.NetworkSettings == nil {
		t.Fatalf("app sandbox container %s has no network settings", externalID)
	}
	bindings := info.NetworkSettings.Ports[nat.Port(fmt.Sprintf("%d/tcp", containerPort))]
	if len(bindings) == 0 || strings.TrimSpace(bindings[0].HostPort) == "" {
		t.Fatalf("app sandbox container %s has no host binding for port %d: %+v", externalID, containerPort, info.NetworkSettings.Ports)
	}
	return bindings[0].HostPort
}

// assertAppsFlagshipSystemdSandbox proves the app sandbox BOOTED WITH
// SYSTEMD (the compose default, HIVY_SANDBOX_DOCKER_SYSTEMD=1 on api+worker):
// the container's entrypoint is exactly /sbin/init (systemd as PID 1) and
// hivy-appd runs as an ACTIVE systemd service — the systemd process-manager
// path is live, not the legacy entrypoint script.
func assertAppsFlagshipSystemdSandbox(t *testing.T, ctx context.Context, db *gorm.DB, app *model.App) {
	t.Helper()
	sb := appsFlagshipLoadSandbox(t, ctx, db, *app.SandboxID)
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("create docker client: %v", err)
	}
	defer cli.Close()
	info, err := cli.ContainerInspect(ctx, sb.ExternalID)
	if err != nil {
		t.Fatalf("inspect app sandbox container %s: %v", sb.ExternalID, err)
	}
	if info.Config == nil || len(info.Config.Entrypoint) != 1 || info.Config.Entrypoint[0] != "/sbin/init" {
		var got []string
		if info.Config != nil {
			got = info.Config.Entrypoint
		}
		t.Fatalf("app sandbox entrypoint=%v want [/sbin/init] — the sandbox did not boot systemd as PID 1", got)
	}
	out, err := exec.CommandContext(ctx, "docker", "exec", sb.ExternalID, "systemctl", "is-active", "hivy-appd").CombinedOutput()
	state := strings.TrimSpace(string(out))
	if err != nil || state != "active" {
		t.Fatalf("systemctl is-active hivy-appd = %q (err=%v) — hivy-appd is not a live systemd service", state, err)
	}
	t.Logf("app sandbox %s booted with systemd PID 1; hivy-appd systemd unit is active", sb.ExternalID)
}

// appsFlagshipObjectStore opens the compose MinIO from the HOST vantage
// point with the same credentials/bucket the api container uses.
func appsFlagshipObjectStore(t *testing.T) *storage.S3Presigner {
	t.Helper()
	endpoint := "http://localhost:" + envOr("HIVY_COMPOSE_MINIO_API_PORT", "9000")
	store, err := storage.NewS3Presigner(storage.PublicAssetsConfig{
		Bucket:    envOr("HIVY_AWS_S3_BUCKET_NAME", "hivy-rag-test"),
		Region:    envOr("HIVY_AWS_REGION", "us-east-1"),
		Endpoint:  endpoint,
		AccessKey: envOr("HIVY_AWS_ACCESS_KEY_ID", "minioadmin"),
		SecretKey: envOr("HIVY_AWS_SECRET_ACCESS_KEY", "minioadmin"),
	})
	if err != nil {
		t.Fatalf("open object store at %s: %v", endpoint, err)
	}
	return store
}

// appsFlagshipObjectSHA downloads an object fully and returns (sha256hex, bytes).
func appsFlagshipObjectSHA(t *testing.T, ctx context.Context, store *storage.S3Presigner, key string) (string, int64) {
	t.Helper()
	body, err := store.Open(ctx, key)
	if err != nil {
		t.Fatalf("open object %s: %v", key, err)
	}
	defer body.Close()
	hasher := sha256.New()
	n, err := io.Copy(hasher, body)
	if err != nil {
		t.Fatalf("read object %s: %v", key, err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), n
}

// appsFlagshipDecryptAppSecret decrypts apps.encrypted_app_secret with the
// stack's HIVY_SANDBOX_ENCRYPTION_KEY — the same key path the internal app
// API uses to verify bearers.
func appsFlagshipDecryptAppSecret(t *testing.T, encrypted []byte) string {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv("HIVY_SANDBOX_ENCRYPTION_KEY"))
	if raw == "" {
		t.Fatalf("HIVY_SANDBOX_ENCRYPTION_KEY is required to verify the app secret")
	}
	key, err := crypto.NewSymmetricKey(raw)
	if err != nil {
		t.Fatalf("load sandbox encryption key: %v", err)
	}
	secret, err := key.DecryptString(encrypted)
	if err != nil {
		t.Fatalf("decrypt app secret: %v", err)
	}
	return secret
}

// appsFlagshipWaitForFinalMarker polls the events API until a final event
// NEWER than afterSeq contains the marker, refreshing the access token both
// proactively and on 401 (long agent turns outlive a 15-minute JWT). A final
// event past afterSeq WITHOUT the marker fails fast — the turn ended some
// other way. afterSeq=0 accepts any final.
func appsFlagshipWaitForFinalMarker(t *testing.T, ctx context.Context, apiBase string, token *string, orgID, sessionID, marker, email, password string, timeout time.Duration, afterSeq int64) agentSessionsEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	nextRefresh := time.Now().Add(10 * time.Minute)
	var lastTypes []string
	for time.Now().Before(deadline) {
		if time.Now().After(nextRefresh) {
			*token = agentSessionsLogin(t, ctx, apiBase, email, password, orgID).AccessToken
			nextRefresh = time.Now().Add(10 * time.Minute)
			t.Logf("proactively refreshed access token")
		}
		status, raw := qaAgentDo(t, ctx, http.MethodGet, apiBase+"/v1/sessions/"+sessionID+"/events?limit=100", *token, orgID, nil)
		if status == http.StatusUnauthorized {
			t.Logf("events API returned 401; re-logging in")
			*token = agentSessionsLogin(t, ctx, apiBase, email, password, orgID).AccessToken
			nextRefresh = time.Now().Add(10 * time.Minute)
			continue
		}
		if status != http.StatusOK {
			t.Fatalf("events poll status=%d body=%s", status, raw)
		}
		var out struct {
			Data []agentSessionsEvent `json:"data"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("decode events poll: %v body=%s", err, raw)
		}
		lastTypes = lastTypes[:0]
		for _, event := range out.Data {
			lastTypes = append(lastTypes, event.EventType)
			if event.EventType != "final" || event.SequenceNumber <= afterSeq {
				continue
			}
			payloadRaw, _ := json.Marshal(event.Payload)
			if strings.Contains(string(payloadRaw), marker) {
				return event
			}
			t.Fatalf("turn ended without marker %s; final payload: %s", marker, payloadRaw)
		}
		t.Logf("waiting for marker=%s event_types=%v", marker, lastTypes)
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for marker %s: %v", marker, ctx.Err())
		case <-time.After(5 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for marker=%s last_event_types=%v", marker, lastTypes)
	return agentSessionsEvent{}
}

// appsFlagshipFindEvent pages through the session's events newest-first and
// returns the first event matching the predicate. Unlike
// agentSessionsListAllEvents it stops as soon as it matches and tolerates
// long transcripts (a full app build produces well over ten pages).
func appsFlagshipFindEvent(t *testing.T, ctx context.Context, apiBase, token, orgID, sessionID string, match func(agentSessionsEvent) bool) *agentSessionsEvent {
	t.Helper()
	cursor := ""
	for page := 0; page < 200; page++ {
		query := url.Values{}
		query.Set("limit", "100")
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		var out struct {
			Data       []agentSessionsEvent `json:"data"`
			NextCursor *string              `json:"next_cursor"`
			HasMore    bool                 `json:"has_more"`
		}
		endpoint := apiBase + "/v1/sessions/" + sessionID + "/events?" + query.Encode()
		agentSessionsJSON(t, ctx, http.MethodGet, endpoint, token, orgID, nil, http.StatusOK, &out)
		for i := range out.Data {
			if match(out.Data[i]) {
				return &out.Data[i]
			}
		}
		if !out.HasMore || out.NextCursor == nil || *out.NextCursor == "" {
			return nil
		}
		cursor = *out.NextCursor
	}
	t.Fatalf("session %s exceeded event pagination safety limit while searching", sessionID)
	return nil
}
