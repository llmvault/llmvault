package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

// --- app_status ----------------------------------------------------------------

type appStatusArgs struct {
	AppID         string `json:"app_id"`
	HivySessionID string `json:"_hivy_session_id"`
}

func registerAppStatus(server *mcp.Server, svc *Service, token *model.Token) {
	server.AddTool(&mcp.Tool{
		Name:        toolAppStatus,
		Description: "Get an app's deployment status: lifecycle status (draft/deploying/running/stopped/failed), live URL, the active version (id, notes, sha), and a live health report from the app's sandbox (process state, active release, /healthz probe). Check it after every app_publish and before handing the user a URL. health_error is set when the sandbox is unreachable or the app was never deployed.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"app_id": map[string]any{"type": "string", "description": "UUID returned by app_create."},
			},
			"required": []string{"app_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args appStatusArgs
		if errResult := decodeAppToolArgs(req, &args); errResult != nil {
			return errResult, nil
		}
		return handleAppStatus(ctx, svc, token, args)
	})
}

func handleAppStatus(ctx context.Context, svc *Service, token *model.Token, args appStatusArgs) (*mcp.CallToolResult, error) {
	tctx, cancel := context.WithTimeout(ctx, appOpsTimeout)
	defer cancel()
	channelID, _, err := svc.appToolSession(tctx, token, args.HivySessionID)
	if err != nil {
		return appToolError(err.Error()), nil
	}
	app, errResult := svc.appForSession(tctx, token, channelID, args.AppID)
	if errResult != nil {
		return errResult, nil
	}
	status, err := svc.Status(tctx, token.OrgID, app.ID)
	if err != nil {
		return appToolError(err.Error()), nil
	}
	url, _ := svc.AppURL(tctx, status.App) // "" (with error) until the app is running
	return appToolJSON(appStatusOutput(status, url))
}

// appStatusOutput builds the app_status tool payload. It is a pure function of
// the resolved status so the marshal contract can be table-tested directly.
//
// health is appd's GET /health body forwarded verbatim as a json.RawMessage.
// The encoding/json marshaler RE-VALIDATES a RawMessage (via compact), so a
// non-empty-but-invalid probe body (e.g. a whitespace-only or non-JSON 200
// from a gateway) makes json.Marshal(out) fail — the exact "failed to
// serialize response" seen in prod. Guard with json.Valid: forward only valid
// JSON, and degrade an invalid body to health_error instead of failing the
// whole call.
func appStatusOutput(status *AppStatus, url string) map[string]any {
	out := map[string]any{
		"status": status.App.Status,
		"name":   status.App.Name,
		"slug":   status.App.Slug,
	}
	if url != "" {
		out["url"] = url
	}
	if status.ActiveVersion != nil {
		out["active_version"] = map[string]any{
			"id":            status.ActiveVersion.ID.String(),
			"notes":         status.ActiveVersion.Notes,
			"bundle_sha256": status.ActiveVersion.BundleSHA256,
			"created_at":    status.ActiveVersion.CreatedAt,
		}
	}
	if len(status.Health) > 0 {
		if json.Valid(status.Health) {
			out["health"] = status.Health
		} else if status.HealthError == "" {
			status.HealthError = "app health probe returned a non-JSON response"
		}
	}
	if status.HealthError != "" {
		out["health_error"] = status.HealthError
	}
	return out
}

// --- app_logs ------------------------------------------------------------------

type appLogsArgs struct {
	AppID         string `json:"app_id"`
	Lines         int    `json:"lines"`
	Grep          string `json:"grep"`
	Since         string `json:"since"`
	HivySessionID string `json:"_hivy_session_id"`
}

func registerAppLogs(server *mcp.Server, svc *Service, token *model.Token) {
	server.AddTool(&mcp.Tool{
		Name:        toolAppLogs,
		Description: "Read an app's recent production logs (the app's stdout/stderr, structured JSON lines from app.Log()). This is your debugger for a live app: it works even when the app sandbox was asleep — the platform wakes it. Filter with grep (a regex, e.g. \"error|panic\") and since (RFC3339) to narrow to the failure window; lines caps how many of the newest matching lines return (hard cap 2000).",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"app_id": map[string]any{"type": "string", "description": "UUID returned by app_create."},
				"lines":  map[string]any{"type": "integer", "minimum": 1, "maximum": 2000, "description": "Max newest lines to return, applied after filtering (default 200, cap 2000)."},
				"grep":   map[string]any{"type": "string", "description": "Regex applied per line before the cap, e.g. \"error|panic\"."},
				"since":  map[string]any{"type": "string", "description": "RFC3339 timestamp; drops lines older than this (lines without a parseable timestamp are kept)."},
			},
			"required": []string{"app_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args appLogsArgs
		if errResult := decodeAppToolArgs(req, &args); errResult != nil {
			return errResult, nil
		}
		return handleAppLogs(ctx, svc, token, args)
	})
}

func handleAppLogs(ctx context.Context, svc *Service, token *model.Token, args appLogsArgs) (*mcp.CallToolResult, error) {
	tctx, cancel := context.WithTimeout(ctx, appOpsTimeout)
	defer cancel()
	channelID, _, err := svc.appToolSession(tctx, token, args.HivySessionID)
	if err != nil {
		return appToolError(err.Error()), nil
	}
	app, errResult := svc.appForSession(tctx, token, channelID, args.AppID)
	if errResult != nil {
		return errResult, nil
	}
	payload, err := svc.Logs(tctx, token.OrgID, app.ID, LogsQuery{
		Lines: args.Lines,
		Grep:  args.Grep,
		Since: args.Since,
	})
	if err != nil {
		return appToolError(err.Error()), nil
	}
	return appToolText(formatAppLogs(payload))
}

// formatAppLogs renders the appd /logs payload as plain text the model can
// read directly: a one-line summary (count, stream, app process state), then
// the raw log lines. An unrecognized payload passes through verbatim.
func formatAppLogs(payload json.RawMessage) string {
	var decoded struct {
		Stream  string          `json:"stream"`
		Lines   []string        `json:"lines"`
		Count   int             `json:"count"`
		App     json.RawMessage `json:"app"`
		Journal []string        `json:"journal"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil || decoded.Stream == "" {
		return string(payload)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d log lines (stream %s)", decoded.Count, decoded.Stream)
	if len(decoded.App) > 0 {
		fmt.Fprintf(&b, "; app process: %s", string(decoded.App))
	}
	b.WriteString("\n")
	if len(decoded.Lines) == 0 {
		b.WriteString("(no log lines matched)")
	} else {
		b.WriteString(strings.Join(decoded.Lines, "\n"))
	}
	if len(decoded.Journal) > 0 {
		b.WriteString("\n--- journal ---\n")
		b.WriteString(strings.Join(decoded.Journal, "\n"))
	}
	return b.String()
}

// --- app_rollback --------------------------------------------------------------

type appRollbackArgs struct {
	AppID         string `json:"app_id"`
	VersionID     string `json:"version_id"`
	HivySessionID string `json:"_hivy_session_id"`
}

func registerAppRollback(server *mcp.Server, svc *Service, token *model.Token) {
	server.AddTool(&mcp.Tool{
		Name:        toolAppRollback,
		Description: "Roll an app back to a previously published version, immediately. Use it first when a new deploy regresses — restore the last good version_id (see app_status active_version and your publish notes), then debug at leisure and republish when fixed. The rollback repoints the app to that version's build; if the sandbox no longer holds it on disk, the version is re-deployed from storage.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"app_id":     map[string]any{"type": "string", "description": "UUID returned by app_create."},
				"version_id": map[string]any{"type": "string", "description": "UUID of the previously published version to restore (returned by app_publish)."},
			},
			"required": []string{"app_id", "version_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args appRollbackArgs
		if errResult := decodeAppToolArgs(req, &args); errResult != nil {
			return errResult, nil
		}
		return handleAppRollback(ctx, svc, token, args)
	})
}

func handleAppRollback(ctx context.Context, svc *Service, token *model.Token, args appRollbackArgs) (*mcp.CallToolResult, error) {
	// Rollback may fall back to a full re-deploy from storage, so it gets the
	// publish budget, not the quick one.
	tctx, cancel := context.WithTimeout(ctx, appPublishTimeout)
	defer cancel()
	channelID, _, err := svc.appToolSession(tctx, token, args.HivySessionID)
	if err != nil {
		return appToolError(err.Error()), nil
	}
	app, errResult := svc.appForSession(tctx, token, channelID, args.AppID)
	if errResult != nil {
		return errResult, nil
	}
	versionID, errResult := parseAppToolUUID(args.VersionID, "version_id")
	if errResult != nil {
		return errResult, nil
	}
	if _, err := svc.Rollback(tctx, token.OrgID, app.ID, versionID); err != nil {
		return appToolError("rollback failed: " + err.Error()), nil
	}
	reloaded, err := svc.GetApp(tctx, token.OrgID, app.ID)
	if err != nil {
		return appToolError(err.Error()), nil
	}
	return appToolJSON(map[string]any{"status": reloaded.Status})
}
