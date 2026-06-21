package canvas

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
)

const SessionTTL = 365 * 24 * time.Hour

type Client struct {
	PublicURL       string
	APIBaseURL      string
	ControlPlaneKey string
	HTTPClient      *http.Client
}

func NewClient(cfg *config.Config) *Client {
	if cfg == nil {
		return nil
	}
	return &Client{
		PublicURL:       strings.TrimRight(strings.TrimSpace(cfg.CanvasPublicURL), "/"),
		APIBaseURL:      strings.TrimRight(strings.TrimSpace(cfg.CanvasAPIBaseURL), "/"),
		ControlPlaneKey: strings.TrimSpace(cfg.CanvasControlPlaneKey),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) Enabled() bool {
	return c != nil &&
		strings.TrimSpace(c.PublicURL) != "" &&
		strings.TrimSpace(c.APIBaseURL) != "" &&
		strings.TrimSpace(c.ControlPlaneKey) != ""
}

type TeamInput struct {
	TeamID uuid.UUID
	HivyID string
	Name   string
}

type TeamResult struct {
	TeamID           uuid.UUID
	HivyID           string
	DefaultProjectID uuid.UUID
}

type ProfileInput struct {
	ProfileID uuid.UUID
	TeamID    uuid.UUID
	HivyID    string
	Email     string
	Fullname  string
}

type ProfileResult struct {
	ProfileID uuid.UUID
	TeamID    uuid.UUID
	HivyID    string
	MCPToken  string
	MCPURL    string
}

type ProjectInput struct {
	ProjectID uuid.UUID
	TeamID    uuid.UUID
	Name      string
}

type ProjectResult struct {
	ProjectID uuid.UUID
	TeamID    uuid.UUID
}

type FileInput struct {
	FileID    uuid.UUID
	ProjectID uuid.UUID
	ProfileID *uuid.UUID
	Name      string
}

type FileResult struct {
	FileID    uuid.UUID
	ProjectID uuid.UUID
	TeamID    uuid.UUID
}

func (c *Client) UpsertTeam(ctx context.Context, input TeamInput) (*TeamResult, error) {
	body := map[string]any{
		"team-id": input.TeamID.String(),
		"hivy-id": input.HivyID,
		"name":    input.Name,
	}
	var raw map[string]any
	if err := c.do(ctx, http.MethodPost, "/teams", body, &raw); err != nil {
		return nil, err
	}
	return &TeamResult{
		TeamID:           uuidField(raw, "team-id", "team_id", "teamId"),
		HivyID:           stringField(raw, "hivy-id", "hivy_id", "hivyId"),
		DefaultProjectID: uuidField(raw, "default-project-id", "default_project_id", "defaultProjectId"),
	}, nil
}

func (c *Client) UpsertProfile(ctx context.Context, input ProfileInput) (*ProfileResult, error) {
	body := map[string]any{
		"profile-id": input.ProfileID.String(),
		"team-id":    input.TeamID.String(),
		"hivy-id":    input.HivyID,
		"email":      input.Email,
		"fullname":   input.Fullname,
	}
	var raw map[string]any
	if err := c.do(ctx, http.MethodPost, "/profiles", body, &raw); err != nil {
		return nil, err
	}
	return &ProfileResult{
		ProfileID: uuidField(raw, "profile-id", "profile_id", "profileId"),
		TeamID:    uuidField(raw, "team-id", "team_id", "teamId"),
		HivyID:    stringField(raw, "hivy-id", "hivy_id", "hivyId"),
		MCPToken:  stringField(raw, "mcp-token", "mcp_token", "mcpToken"),
		MCPURL:    stringField(raw, "mcp-url", "mcp_url", "mcpUrl"),
	}, nil
}

func (c *Client) UpsertProject(ctx context.Context, input ProjectInput) (*ProjectResult, error) {
	body := map[string]any{
		"project-id": input.ProjectID.String(),
		"team-id":    input.TeamID.String(),
		"name":       input.Name,
	}
	var raw map[string]any
	if err := c.do(ctx, http.MethodPost, "/projects", body, &raw); err != nil {
		return nil, err
	}
	return &ProjectResult{
		ProjectID: uuidField(raw, "project-id", "project_id", "projectId"),
		TeamID:    uuidField(raw, "team-id", "team_id", "teamId"),
	}, nil
}

func (c *Client) UpsertFile(ctx context.Context, input FileInput) (*FileResult, error) {
	body := map[string]any{
		"file-id":    input.FileID.String(),
		"project-id": input.ProjectID.String(),
		"name":       input.Name,
	}
	if input.ProfileID != nil {
		body["profile-id"] = input.ProfileID.String()
	}
	var raw map[string]any
	if err := c.do(ctx, http.MethodPost, "/files", body, &raw); err != nil {
		return nil, err
	}
	return &FileResult{
		FileID:    uuidField(raw, "file-id", "file_id", "fileId"),
		ProjectID: uuidField(raw, "project-id", "project_id", "projectId"),
		TeamID:    uuidField(raw, "team-id", "team_id", "teamId"),
	}, nil
}

func (c *Client) MintSessionJWT(profileID, teamID uuid.UUID, fileID, pageID *uuid.UUID) (string, error) {
	if !c.Enabled() {
		return "", fmt.Errorf("canvas client is not configured")
	}
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iss":        "hivy",
		"aud":        "penpot-canvas",
		"iat":        now.Unix(),
		"exp":        now.Add(SessionTTL).Unix(),
		"profile_id": profileID.String(),
		"team_id":    teamID.String(),
	}
	if fileID != nil && *fileID != uuid.Nil {
		claims["file_id"] = fileID.String()
	}
	if pageID != nil && *pageID != uuid.Nil {
		claims["page_id"] = pageID.String()
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(c.ControlPlaneKey))
}

func (c *Client) SessionURL(token string) string {
	if c == nil {
		return ""
	}
	return c.PublicURL + "/api/hivy/session?token=" + url.QueryEscape(token)
}

func (c *Client) WorkspaceURL(teamID, fileID uuid.UUID, pageID *uuid.UUID) string {
	if c == nil {
		return ""
	}
	values := url.Values{}
	values.Set("team-id", teamID.String())
	values.Set("file-id", fileID.String())
	if pageID != nil && *pageID != uuid.Nil {
		values.Set("page-id", pageID.String())
	}
	return c.PublicURL + "/#/workspace?" + values.Encode()
}

func (c *Client) do(ctx context.Context, method, path string, payload any, out any) error {
	if !c.Enabled() {
		return fmt.Errorf("canvas client is not configured")
	}
	var body io.Reader
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal canvas request: %w", err)
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint(path), body)
	if err != nil {
		return fmt.Errorf("build canvas request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.ControlPlaneKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("canvas request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("canvas request %s %s failed with %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if readErr != nil {
		return fmt.Errorf("read canvas response: %w", readErr)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode canvas response: %w", err)
		}
	}
	return nil
}

func (c *Client) endpoint(path string) string {
	base := strings.TrimRight(c.APIBaseURL, "/")
	if !strings.HasSuffix(base, "/api/hivy") {
		base += "/api/hivy"
	}
	return base + path
}

func stringField(raw map[string]any, names ...string) string {
	for _, name := range names {
		if value, ok := raw[name]; ok {
			if s, ok := value.(string); ok {
				return s
			}
		}
	}
	return ""
}

func uuidField(raw map[string]any, names ...string) uuid.UUID {
	if s := stringField(raw, names...); s != "" {
		if id, err := uuid.Parse(s); err == nil {
			return id
		}
	}
	return uuid.Nil
}
