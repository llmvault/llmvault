package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/mcpserver"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sheets"
	"github.com/usehivy/hivy/internal/skills"
)

// sheetsE2ERedisAddr resolves the compose Redis address reachable from the
// host (mirrors agentSessionsDatabaseURL for Postgres).
func sheetsE2ERedisAddr() string {
	if value := strings.TrimSpace(os.Getenv("AGENT_SESSIONS_E2E_REDIS_ADDR")); value != "" {
		return value
	}
	port := strings.TrimSpace(os.Getenv("HIVY_COMPOSE_REDIS_PORT"))
	if port == "" {
		port = "16379"
	}
	return "localhost:" + port
}

// sheetsE2ESpyEnqueuer records every job handed to the import enqueuer and
// delegates to the real Asynq-backed enqueuer so the live worker still runs
// the job.
type sheetsE2ESpyEnqueuer struct {
	inner sheets.ImportEnqueuer
	mu    sync.Mutex
	jobs  []uuid.UUID
}

func (s *sheetsE2ESpyEnqueuer) EnqueueSheetCSVImport(ctx context.Context, jobID uuid.UUID) error {
	s.mu.Lock()
	s.jobs = append(s.jobs, jobID)
	s.mu.Unlock()
	return s.inner.EnqueueSheetCSVImport(ctx, jobID)
}

func (s *sheetsE2ESpyEnqueuer) saw(jobID uuid.UUID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range s.jobs {
		if id == jobID {
			return true
		}
	}
	return false
}

func (s *sheetsE2ESpyEnqueuer) jobIDs() []uuid.UUID {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]uuid.UUID(nil), s.jobs...)
}

// sheetsE2EConnectMCP builds the MCP server through the production
// mcpserver.BuildServer path (skills + sheets tool funcs, exactly as serve.go
// wires them) for an agent-proxy token, and returns a connected in-memory
// client session.
func sheetsE2EConnectMCP(t *testing.T, ctx context.Context, db *gorm.DB, svc *sheets.Service, orgID, agentID uuid.UUID) *mcp.ClientSession {
	t.Helper()
	token := &model.Token{
		OrgID: orgID,
		Meta: model.JSON{
			model.TokenMetaType:    model.TokenTypeAgentProxy,
			model.TokenMetaAgentID: agentID.String(),
		},
	}
	server, err := mcpserver.BuildServer(ctx, token, db, nil, nil, nil, nil, nil, skills.NewToolsFunc(db, "http://localhost:3000"), nil, sheets.NewToolsFunc(svc), nil, nil) //nolint:contextcheck // tool handlers receive their own request context from the MCP server at call time.
	if err != nil {
		t.Fatalf("build MCP server: %v", err)
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("connect MCP server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "sheets-e2e", Version: "v1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect MCP client: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func sheetsE2EListTools(t *testing.T, ctx context.Context, client *mcp.ClientSession) map[string]*mcp.Tool {
	t.Helper()
	tools, err := client.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list MCP tools: %v", err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tool := range tools.Tools {
		byName[tool.Name] = tool
	}
	return byName
}

func sheetsE2EToolNameList(tools map[string]*mcp.Tool) []string {
	out := make([]string, 0, len(tools))
	for name := range tools {
		out = append(out, name)
	}
	return out
}

// sheetsE2ESessionID, when set by the test, is injected as the runtime-controlled
// _hivy_session_id into every tool call — sheets tools are channel-scoped and
// derive their channel from the session. In production the Rust runtime injects
// this; the e2e harness stands in for the runtime here.
var sheetsE2ESessionID string

func sheetsE2ECallTool(t *testing.T, ctx context.Context, client *mcp.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	if sheetsE2ESessionID != "" {
		if args == nil {
			args = map[string]any{}
		}
		if _, ok := args["_hivy_session_id"]; !ok {
			args["_hivy_session_id"] = sheetsE2ESessionID
		}
	}
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	var text []string
	for _, item := range result.Content {
		if tc, ok := item.(*mcp.TextContent); ok {
			text = append(text, tc.Text)
		}
	}
	joined := strings.Join(text, "\n")
	if result.IsError {
		t.Fatalf("%s returned error: %s", name, joined)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(joined), &out); err != nil {
		t.Fatalf("decode %s response %q: %v", name, joined, err)
	}
	return out
}

func sheetsE2ECallToolJSON(t *testing.T, ctx context.Context, client *mcp.ClientSession, name, payload string) map[string]any {
	t.Helper()
	var args map[string]any
	if err := json.Unmarshal([]byte(payload), &args); err != nil {
		t.Fatalf("payload for %s is not valid JSON: %v\n%s", name, err, payload)
	}
	return sheetsE2ECallTool(t, ctx, client, name, args)
}

func sheetsE2ESkillListed(out map[string]any, slug string) bool {
	items, _ := out["skills"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if name, _ := item["name"].(string); strings.EqualFold(name, slug) {
			return true
		}
	}
	return false
}

// sheetsE2EUploadDriveCSV uploads a CSV exactly as a sandbox agent does:
// PUT /internal/agents/{agentID}/sandboxes/{sandboxID}/drive/{path} with the
// sandbox's runtime secret as the bearer (the SKILL.md walkthrough's
// $HIVY_DRIVE_UPLOAD_URL flow). Returns the pub/e/{agentID}/… object key.
func sheetsE2EUploadDriveCSV(t *testing.T, ctx context.Context, apiBase, agentID, sandboxID, runtimeSecret, path, content string) string {
	t.Helper()
	url := apiBase + "/internal/agents/" + agentID + "/sandboxes/" + sandboxID + "/drive/" + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, strings.NewReader(content))
	if err != nil {
		t.Fatalf("build drive upload request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+runtimeSecret)
	req.Header.Set("Content-Type", "text/csv")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("drive upload: %v", err)
	}
	defer resp.Body.Close()
	var decoded struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil || resp.StatusCode != http.StatusCreated || decoded.Key == "" {
		t.Fatalf("drive upload status=%d key=%q err=%v", resp.StatusCode, decoded.Key, err)
	}
	return decoded.Key
}

// sheetsE2EUploadCSV uploads a CSV through the server-side upload proxy
// (POST /v1/uploads/upload, asset_type sheet_import) and returns the object
// key — the same path the web app uses in local dev.
func sheetsE2EUploadCSV(t *testing.T, ctx context.Context, apiBase, token, orgID, filename, content string) string {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if err := writer.WriteField("asset_type", "sheet_import"); err != nil {
		t.Fatalf("write asset_type field: %v", err)
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	header.Set("Content-Type", "text/csv")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("create multipart file part: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("write csv body: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/v1/uploads/upload", &buf)
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-ID", orgID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload csv: %v", err)
	}
	defer resp.Body.Close()
	var decoded struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil || resp.StatusCode != http.StatusOK || decoded.Key == "" {
		t.Fatalf("upload csv status=%d key=%q err=%v", resp.StatusCode, decoded.Key, err)
	}
	return decoded.Key
}

type sheetsE2EImportJob struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	TotalRows     int64  `json:"total_rows"`
	ProcessedRows int64  `json:"processed_rows"`
	Error         string `json:"error"`
}

// sheetsE2EWaitForImport polls the REST import status endpoint until the live
// worker finishes the job.
func sheetsE2EWaitForImport(t *testing.T, ctx context.Context, apiBase, token, orgID, jobID string) sheetsE2EImportJob {
	t.Helper()
	deadline := time.Now().Add(3 * time.Minute)
	var job sheetsE2EImportJob
	for time.Now().Before(deadline) {
		agentSessionsJSON(t, ctx, http.MethodGet, apiBase+"/v1/sheets/imports/"+jobID, token, orgID, nil, http.StatusOK, &job)
		switch job.Status {
		case "completed":
			t.Logf("import %s completed processed=%d total=%d", jobID, job.ProcessedRows, job.TotalRows)
			return job
		case "failed":
			t.Fatalf("import %s failed: %s", jobID, job.Error)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context ended waiting for import %s: %v", jobID, ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("import %s did not complete: %+v", jobID, job)
	return job
}
