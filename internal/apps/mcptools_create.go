package apps

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/model"
)

// --- app_create ----------------------------------------------------------------

type appCreateArgs struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	Icon            string `json:"icon"`
	SheetID         string `json:"sheet_id"`
	HivySessionID   string `json:"_hivy_session_id"`
	HivyActorUserID string `json:"_hivy_actor_user_id"`
}

func registerAppCreate(server *mcp.Server, svc *Service, token *model.Token, agentID uuid.UUID) {
	server.AddTool(&mcp.Tool{
		Name:        toolAppCreate,
		Description: "Create a new Hivy app bound to exactly ONE sheet in the current channel. The binding is permanent — the app reads that sheet's structure and does row CRUD on it, never schema changes. Returns app_id (used by every other app tool) and slug (names your workspace directory and drive upload folder). Call sheet_describe first to confirm the sheet and capture its field IDs; then follow the apps skill workflow to build and publish.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"name":        map[string]any{"type": "string", "description": "App display name, e.g. \"Leads Manager\". The app's slug is derived from it and must be unique in the org."},
				"description": map[string]any{"type": "string", "description": "Short description of what the app does."},
				"icon":        map[string]any{"type": "string", "description": "Optional icon name shown on the app card."},
				"sheet_id":    map[string]any{"type": "string", "description": "UUID of the sheet to bind — must be a sheet in the channel you are working in."},
			},
			"required": []string{"name", "sheet_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args appCreateArgs
		if errResult := decodeAppToolArgs(req, &args); errResult != nil {
			return errResult, nil
		}
		return handleAppCreate(ctx, svc, token, agentID, args)
	})
}

func handleAppCreate(ctx context.Context, svc *Service, token *model.Token, agentID uuid.UUID, args appCreateArgs) (*mcp.CallToolResult, error) {
	tctx, cancel := context.WithTimeout(ctx, appToolTimeout)
	defer cancel()
	if strings.TrimSpace(args.Name) == "" {
		return appToolError("name is required"), nil
	}
	sheetID, errResult := parseAppToolUUID(args.SheetID, "sheet_id")
	if errResult != nil {
		return errResult, nil
	}
	channelID, sessionID, err := svc.appToolSession(tctx, token, args.HivySessionID)
	if err != nil {
		return appToolError(err.Error()), nil
	}
	params := CreateAppParams{
		OrgID:            token.OrgID,
		ChannelID:        channelID,
		SheetID:          sheetID,
		Name:             args.Name,
		Description:      args.Description,
		Icon:             args.Icon,
		CreatedByAgentID: &agentID,
		SourceSessionID:  &sessionID,
	}
	// The runtime-injected _hivy_actor_user_id (never model-visible) is the
	// human behind the turn; record them as the app's creating user when
	// present and valid in this org.
	actor, err := access.Resolve(tctx, svc.db, token.OrgID, args.HivyActorUserID)
	if err != nil {
		return appToolError(err.Error()), nil
	}
	if actor != nil {
		params.CreatedByUserID = &actor.UserID
	}
	app, err := svc.CreateApp(tctx, params)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return appToolError("sheet not found in this channel — sheet_id must be an existing sheet in the channel you are working in (use sheet_list)"), nil
		}
		return appToolError(err.Error()), nil
	}
	return appToolJSON(map[string]any{
		"app_id": app.ID.String(),
		"slug":   app.Slug,
		"status": app.Status,
	})
}

// --- app_publish ---------------------------------------------------------------

type appPublishArgs struct {
	AppID         string `json:"app_id"`
	SourceKey     string `json:"source_key"`
	BundleKey     string `json:"bundle_key"`
	SourceSHA256  string `json:"source_sha256"`
	BundleSHA256  string `json:"bundle_sha256"`
	Notes         string `json:"notes"`
	HivySessionID string `json:"_hivy_session_id"`
}

func registerAppPublish(server *mcp.Server, svc *Service, token *model.Token, agentID uuid.UUID) {
	server.AddTool(&mcp.Tool{
		Name:        toolAppPublish,
		Description: "Publish a new version of an app and deploy it, synchronously — this call copies your uploaded zips into immutable version storage, provisions the app sandbox if needed, and restarts the app on the new build. It can take a minute or two; when it returns successfully the app is live at the returned url. Upload dist/source.zip and dist/bundle.zip to your drive first and pass the returned keys plus their sha256s (verified on copy). Always pass notes — it is the version's changelog and what makes app_rollback targets identifiable. On failure, use app_logs to debug, fix, and republish.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"app_id":        map[string]any{"type": "string", "description": "UUID returned by app_create."},
				"source_key":    map[string]any{"type": "string", "description": "Drive object key of the uploaded source.zip (pub/e/… key returned by the drive PUT)."},
				"bundle_key":    map[string]any{"type": "string", "description": "Drive object key of the uploaded bundle.zip."},
				"source_sha256": map[string]any{"type": "string", "description": "Hex sha256 of source.zip (64 chars)."},
				"bundle_sha256": map[string]any{"type": "string", "description": "Hex sha256 of bundle.zip (64 chars)."},
				"notes":         map[string]any{"type": "string", "description": "Changelog for this version — what changed and why. Strongly recommended on every publish."},
			},
			"required": []string{"app_id", "source_key", "bundle_key", "source_sha256", "bundle_sha256"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args appPublishArgs
		if errResult := decodeAppToolArgs(req, &args); errResult != nil {
			return errResult, nil
		}
		return handleAppPublish(ctx, svc, token, agentID, args)
	})
}

func handleAppPublish(ctx context.Context, svc *Service, token *model.Token, agentID uuid.UUID, args appPublishArgs) (*mcp.CallToolResult, error) {
	// Publish is deliberately synchronous (copy → version → sandbox → deploy)
	// and may take minutes; bound it generously rather than with the quick
	// tool timeout.
	tctx, cancel := context.WithTimeout(ctx, appPublishTimeout)
	defer cancel()
	channelID, sessionID, err := svc.appToolSession(tctx, token, args.HivySessionID)
	if err != nil {
		return appToolError(err.Error()), nil
	}
	app, errResult := svc.appForSession(tctx, token, channelID, args.AppID)
	if errResult != nil {
		return errResult, nil
	}
	version, err := svc.Publish(tctx, PublishParams{
		OrgID:           token.OrgID,
		AppID:           app.ID,
		SourceKey:       args.SourceKey,
		BundleKey:       args.BundleKey,
		SourceSHA256:    args.SourceSHA256,
		BundleSHA256:    args.BundleSHA256,
		Notes:           args.Notes,
		ActorAgentID:    &agentID,
		SourceSessionID: &sessionID,
	})
	if err != nil {
		if version != nil {
			// The version row exists but the deploy failed — the app is in
			// status "failed". Point the model at its debugger.
			return appToolError("deploy failed: " + err.Error() + ". The version was recorded but the app is not serving it; use app_logs to inspect the failure, fix, and publish again."), nil
		}
		return appToolError("publish failed: " + err.Error()), nil
	}
	reloaded, err := svc.GetApp(tctx, token.OrgID, app.ID)
	if err != nil {
		return appToolError(err.Error()), nil
	}
	url, err := svc.AppURL(tctx, reloaded)
	if err != nil {
		return appToolError("published, but resolving the app URL failed: " + err.Error()), nil
	}
	return appToolJSON(map[string]any{
		"version_id": version.ID.String(),
		"url":        appLaunchURL(url, app.ID),
		"status":     reloaded.Status,
	})
}

// appLaunchURL appends the ?app=<appID> query hint the frontend uses to detect
// a launchable app link. The param name is the frontend contract — keep it "app".
func appLaunchURL(rawURL string, appID uuid.UUID) string {
	if rawURL == "" {
		return rawURL
	}
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + "app=" + appID.String()
}
