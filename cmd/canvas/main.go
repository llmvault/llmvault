package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	version = "dev"
	commit  = "unknown"
)

const (
	defaultStateDir  = "/workspace/.hivy/canvas"
	defaultCanvasDir = "/workspace/canvas"

	envControlPlaneURL = "HIVY_CONTROL_PLANE_URL"
	envAgentID         = "HIVY_AGENT_ID"
	envRuntimeSecret   = "HIVY_RUNTIME_SECRET" // #nosec G101 -- environment variable name.
	envCanvasWorkspace = "CANVAS_WORKSPACE_DIR"
)

type cliState struct {
	ProjectID string `json:"project_id,omitempty"`
	UpdatedAt string `json:"updated_at"`
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
	case "project":
		return projectCommand(args[1:])
	case "artifact":
		return artifactCommand(args[1:])
	case "brands", "brand":
		return brandsCommand(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	name := cliName()
	_ = writeStdout(fmt.Sprintf(`%s controls a Hivy-managed Canvas session.

Usage:
  %[1]s doctor
  %[1]s project list
  %[1]s project create --name "Project"
  %[1]s artifact create --project <slug-or-id> --type web_page|presentation --name "Artifact"
  %[1]s artifact list --project <slug-or-id>
  %[1]s artifact validate <artifact-path>
  %[1]s artifact verify <artifact-path>
  %[1]s artifact sync <artifact-path>
  %[1]s brands list
  %[1]s brands view <brand-id>
  %[1]s brands create --name "Brand" [--json '{"colors":{...}}']
  %[1]s brands update <brand-id> --json '{"description":"..."}'
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
	if len(missing) > 0 {
		return fmt.Errorf("canvas runtime not ready: missing %s", strings.Join(missing, ", "))
	}
	return writeStdout(fmt.Sprintf("%s runtime ok\n", cliName()))
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

func stateDir() string {
	if dir := strings.TrimSpace(os.Getenv("CANVAS_CLI_STATE_DIR")); dir != "" {
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
