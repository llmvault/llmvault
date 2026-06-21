package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	version = "dev"
	commit  = "unknown"
)

const (
	browserSession  = "canvas"
	defaultStateDir = "/workspace/.hivy/canvas"

	envCanvasURL        = "PENPOT_CANVAS_URL"
	envCanvasTeamID     = "PENPOT_CANVAS_TEAM_ID"
	envCanvasProfileID  = "PENPOT_CANVAS_PROFILE_ID"
	envCanvasSessionJWT = "PENPOT_CANVAS_SESSION_JWT" // #nosec G101 -- environment variable name.
	envCanvasMCPURL     = "PENPOT_CANVAS_MCP_URL"
	envControlPlaneURL  = "HIVY_CONTROL_PLANE_URL"
	envAgentID          = "HIVY_AGENT_ID"
	envRuntimeSecret    = "HIVY_RUNTIME_SECRET" // #nosec G101 -- environment variable name.
)

type cliState struct {
	ProjectID    string `json:"project_id,omitempty"`
	FileID       string `json:"file_id,omitempty"`
	PageID       string `json:"page_id,omitempty"`
	WorkspaceURL string `json:"workspace_url,omitempty"`
	UpdatedAt    string `json:"updated_at"`
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(os.Stderr, cliName()+":", r)
			os.Exit(1)
		}
	}()
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, cliName()+":", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	switch args[0] {
	case "--help", "-h", "help":
		usage()
		return nil
	case "--version", "-v", "version":
		return writeStdout(fmt.Sprintf("%s %s (%s)\n", cliName(), version, commit))
	case "doctor":
		return doctor()
	case "init":
		return initCanvas()
	case "project":
		return projectCommand(args[1:])
	case "file":
		return fileCommand(args[1:])
	case "mcp":
		return mcpCommand(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	name := cliName()
	_ = writeStdout(fmt.Sprintf(`%s controls a Hivy-managed Canvas session.

Usage:
  %[1]s doctor
  %[1]s init
  %[1]s project list
  %[1]s project create --name "Project"
  %[1]s file list
  %[1]s file create --name "File" --project-id <canvas-project-id>
  %[1]s file switch <canvas-file-id> [--page-id <page-id>]
  %[1]s file current
  %[1]s mcp <tool> --json '{"key":"value"}'
`, name))
}

func cliName() string {
	name := strings.TrimSpace(filepath.Base(os.Args[0]))
	if name == "" {
		return "canvas"
	}
	return name
}

func doctor() error {
	required := []string{
		envCanvasURL,
		envCanvasTeamID,
		envCanvasProfileID,
		envCanvasSessionJWT,
		envCanvasMCPURL,
		envControlPlaneURL,
		envAgentID,
		envRuntimeSecret,
	}
	var missing []string
	missingConfig := false
	for _, key := range required {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			missingConfig = true
		}
	}
	if missingConfig {
		missing = append(missing, "canvas runtime configuration")
	}
	if _, err := exec.LookPath("browser"); err != nil {
		missing = append(missing, "browser")
	}
	if len(missing) > 0 {
		return fmt.Errorf("canvas runtime not ready: missing %s", strings.Join(missing, ", "))
	}
	return writeStdout(fmt.Sprintf("%s runtime ok\n", cliName()))
}

func initCanvas() error {
	canvasURL := mustEnv(envCanvasURL)
	token := mustEnv(envCanvasSessionJWT)
	sessionURL := strings.TrimRight(canvasURL, "/") + "/api/hivy/session?token=" + url.QueryEscape(token)
	return browserOpen(sessionURL)
}

func projectCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("project subcommand is required")
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("project list", flag.ContinueOnError)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return errors.New("project list does not accept arguments")
		}
		var result map[string]any
		if err := getControlPlane("/internal/agents/"+mustEnv(envAgentID)+"/canvas/projects", &result); err != nil {
			return err
		}
		return printJSON(result)
	case "create":
		fs := flag.NewFlagSet("project create", flag.ContinueOnError)
		name := fs.String("name", "", "project name")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*name) == "" {
			return errors.New("--name is required")
		}
		var result map[string]any
		if err := postControlPlane("/internal/agents/"+mustEnv(envAgentID)+"/canvas/projects", map[string]string{"name": *name}, &result); err != nil {
			return err
		}
		state, _ := loadState()
		state.ProjectID = stringMapField(result, "project_id")
		_ = saveState(state)
		return printJSON(result)
	default:
		return fmt.Errorf("unknown project subcommand %q", args[0])
	}
}

func fileCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("file subcommand is required")
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("file list", flag.ContinueOnError)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return errors.New("file list does not accept arguments")
		}
		var result map[string]any
		if err := getControlPlane("/internal/agents/"+mustEnv(envAgentID)+"/canvas/files", &result); err != nil {
			return err
		}
		return printJSON(result)
	case "create":
		fs := flag.NewFlagSet("file create", flag.ContinueOnError)
		name := fs.String("name", "", "file name")
		projectID := fs.String("project-id", "", "canvas project id")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*name) == "" {
			return errors.New("--name is required")
		}
		if strings.TrimSpace(*projectID) == "" {
			return errors.New("--project-id is required")
		}
		var result map[string]any
		body := map[string]string{"name": *name, "project_id": *projectID}
		if err := postControlPlane("/internal/agents/"+mustEnv(envAgentID)+"/canvas/files", body, &result); err != nil {
			return err
		}
		state, _ := loadState()
		state.ProjectID = stringMapField(result, "project_id")
		state.FileID = stringMapField(result, "file_id")
		state.PageID = stringMapField(result, "page_id")
		state.WorkspaceURL = stringMapField(result, "workspace_url")
		_ = saveState(state)
		return printJSON(result)
	case "switch":
		fs := flag.NewFlagSet("file switch", flag.ContinueOnError)
		pageID := fs.String("page-id", "", "page id")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() < 1 {
			return errors.New("file id is required")
		}
		fileID := fs.Arg(0)
		state, _ := loadState()
		if strings.TrimSpace(*pageID) == "" {
			*pageID = state.PageID
		}
		targetURL := workspaceURL(fileID, *pageID)
		if err := browserOpen(targetURL); err != nil {
			return err
		}
		state.FileID = fileID
		state.PageID = strings.TrimSpace(*pageID)
		state.WorkspaceURL = targetURL
		return saveState(state)
	case "current":
		state, err := loadState()
		if err != nil {
			return err
		}
		if currentURL, err := browserGetURL(); err == nil && strings.TrimSpace(currentURL) != "" {
			state.WorkspaceURL = strings.TrimSpace(currentURL)
		}
		return printJSON(state)
	default:
		return fmt.Errorf("unknown file subcommand %q", args[0])
	}
}

func mcpCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("mcp tool name is required")
	}
	tool := args[0]
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	rawJSON := fs.String("json", "{}", "tool arguments JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	var params any
	if err := json.Unmarshal([]byte(*rawJSON), &params); err != nil {
		return fmt.Errorf("invalid --json: %w", err)
	}
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      fmt.Sprintf("%d", time.Now().UnixNano()),
		"method":  "tools/call",
		"params": map[string]any{
			"name":      tool,
			"arguments": params,
		},
	}
	endpoint := normalizeMCPURL(mustEnv(envCanvasMCPURL))
	sessionID, err := initializeMCP(endpoint)
	if err != nil {
		return err
	}
	var response any
	if _, err := postMCP(endpoint, sessionID, request, &response); err != nil {
		return err
	}
	return printJSON(response)
}

func initializeMCP(endpoint string) (string, error) {
	initialize := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "hivy-canvas-cli",
				"version": version,
			},
		},
	}
	var initResponse any
	sessionID, err := postMCP(endpoint, "", initialize, &initResponse)
	if err != nil {
		return "", err
	}
	if sessionID == "" {
		return "", errors.New("mcp initialize did not return a session id")
	}
	initialized := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}
	if _, err := postMCP(endpoint, sessionID, initialized, nil); err != nil {
		return "", err
	}
	return sessionID, nil
}

func postControlPlane(path string, payload any, out any) error {
	base := strings.TrimRight(mustEnv(envControlPlaneURL), "/")
	return requestJSON(http.MethodPost, base+path, mustEnv(envRuntimeSecret), payload, out)
}

func getControlPlane(path string, out any) error {
	base := strings.TrimRight(mustEnv(envControlPlaneURL), "/")
	return requestJSON(http.MethodGet, base+path, mustEnv(envRuntimeSecret), nil, out)
}

func requestJSON(method, targetURL, bearer string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, targetURL, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%s %s failed with %d: %s", method, targetURL, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if readErr != nil {
		return readErr
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return err
		}
	}
	return nil
}

func postMCP(targetURL, sessionID string, payload any, out any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("POST %s failed with %d: %s", targetURL, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if readErr != nil {
		return "", readErr
	}
	if out != nil && len(data) > 0 {
		if err := decodeMCPBody(resp.Header.Get("Content-Type"), data, out); err != nil {
			return "", err
		}
	}
	return resp.Header.Get("Mcp-Session-Id"), nil
}

func decodeMCPBody(contentType string, data []byte, out any) error {
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "" || payload == "[DONE]" {
				continue
			}
			return json.Unmarshal([]byte(payload), out)
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		return errors.New("mcp event stream did not include a data payload")
	}
	return json.Unmarshal(data, out)
}

func normalizeMCPURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return raw
	}
	if parsed.Path == "" {
		parsed.Path = "/mcp"
	}
	return parsed.String()
}

func browserOpen(targetURL string) error {
	cmd := browserCommand("open", targetURL)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func browserGetURL() (string, error) {
	var out bytes.Buffer
	cmd := browserCommand("get", "url")
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	err := cmd.Run()
	return out.String(), err
}

func browserCommand(args ...string) *exec.Cmd {
	fullArgs := append([]string{"--session", browserSession}, args...)
	cmd := exec.Command("browser", fullArgs...)
	cmd.Env = append(os.Environ(), "AGENT_BROWSER_SESSION="+browserSession)
	return cmd
}

func workspaceURL(fileID, pageID string) string {
	values := url.Values{}
	values.Set("team-id", mustEnv(envCanvasTeamID))
	values.Set("file-id", strings.TrimSpace(fileID))
	if strings.TrimSpace(pageID) != "" {
		values.Set("page-id", strings.TrimSpace(pageID))
	}
	return strings.TrimRight(mustEnv(envCanvasURL), "/") + "/#/workspace?" + values.Encode()
}

func stateDir() string {
	if dir := strings.TrimSpace(os.Getenv("PENPOT_CLI_STATE_DIR")); dir != "" {
		return dir
	}
	return defaultStateDir
}

func statePath() string {
	return filepath.Join(stateDir(), "state.json")
}

func loadState() (cliState, error) {
	var state cliState
	data, err := os.ReadFile(statePath())
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	return state, nil
}

func saveState(state cliState) error {
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(), append(data, '\n'), 0o600)
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeStdout(value string) error {
	_, err := io.WriteString(os.Stdout, value)
	return err
}

func mustEnv(key string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		panic(fmt.Sprintf("%s is required", key))
	}
	return value
}

func stringMapField(values map[string]any, key string) string {
	if value, ok := values[key]; ok {
		if s, ok := value.(string); ok {
			return s
		}
	}
	return ""
}
