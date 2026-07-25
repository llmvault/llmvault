package devseed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultAPIURL = "http://api:8080"
)

var (
	ErrDisabled       = errors.New("development seed is disabled")
	ErrNotDevelopment = errors.New("development seed requires HIVY_ENVIRONMENT=development")
)

// Config describes the minimal local workspace reconciled by the development
// seed command.
type Config struct {
	Enabled         bool
	Environment     string
	APIURL          string
	Email           string
	Password        string
	UserName        string
	OrgName         string
	PromptCompany   string
	TeamName        string
	TeamDescription string
	HTTPClient      *http.Client
}

// Result identifies the local workspace after reconciliation.
type Result struct {
	Email       string
	OrgID       string
	OrgName     string
	TeamID      string
	TeamName    string
	UserCreated bool
	TeamCreated bool
}

// OnboardingStore performs the one development-only state transition that
// cannot go through the product API without a configured connection.
type OnboardingStore interface {
	CompleteOnboarding(ctx context.Context, orgID string) error
}

// Reconcile creates the local account and its first team through the public
// Hivy API, then marks that exact development org ready for immediate use.
func Reconcile(ctx context.Context, cfg Config, store OnboardingStore) (Result, error) {
	if !cfg.Enabled {
		return Result{}, ErrDisabled
	}
	if strings.TrimSpace(cfg.Environment) != "development" {
		return Result{}, ErrNotDevelopment
	}
	if store == nil {
		return Result{}, fmt.Errorf("onboarding store is required")
	}
	if err := validateConfig(&cfg); err != nil {
		return Result{}, err
	}

	client := &apiClient{
		baseURL:    strings.TrimRight(cfg.APIURL, "/"),
		httpClient: cfg.HTTPClient,
	}
	auth, userCreated, err := client.registerOrLogin(ctx, cfg)
	if err != nil {
		return Result{}, err
	}
	if len(auth.Orgs) == 0 || strings.TrimSpace(auth.Orgs[0].ID) == "" {
		return Result{}, fmt.Errorf("development account has no organization")
	}
	orgID := auth.Orgs[0].ID
	client.accessToken = auth.AccessToken
	client.orgID = orgID

	if err := client.updateOrg(ctx, cfg.OrgName, cfg.PromptCompany); err != nil {
		return Result{}, err
	}
	team, teamCreated, err := client.ensureTeam(ctx, cfg.TeamName, cfg.TeamDescription)
	if err != nil {
		return Result{}, err
	}
	if err := store.CompleteOnboarding(ctx, orgID); err != nil {
		return Result{}, fmt.Errorf("complete development onboarding: %w", err)
	}

	return Result{
		Email:       cfg.Email,
		OrgID:       orgID,
		OrgName:     cfg.OrgName,
		TeamID:      team.ID,
		TeamName:    team.Name,
		UserCreated: userCreated,
		TeamCreated: teamCreated,
	}, nil
}

func validateConfig(cfg *Config) error {
	if strings.TrimSpace(cfg.APIURL) == "" {
		cfg.APIURL = defaultAPIURL
	}
	parsed, err := url.Parse(cfg.APIURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("development seed API URL must be absolute")
	}
	required := map[string]string{
		"email": cfg.Email, "password": cfg.Password, "user name": cfg.UserName,
		"organization name": cfg.OrgName, "team name": cfg.TeamName,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if len(cfg.Password) < 8 {
		return fmt.Errorf("development seed password must be at least 8 characters")
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 20 * time.Second}
	}
	return nil
}

type apiClient struct {
	baseURL     string
	accessToken string
	orgID       string
	httpClient  *http.Client
}

type authResponse struct {
	AccessToken string `json:"access_token"`
	Orgs        []struct {
		ID string `json:"id"`
	} `json:"orgs"`
}

type teamResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *apiClient) registerOrLogin(ctx context.Context, cfg Config) (authResponse, bool, error) {
	var auth authResponse
	status, err := c.doJSON(ctx, http.MethodPost, "/auth/register", map[string]string{
		"email": cfg.Email, "password": cfg.Password, "name": cfg.UserName,
	}, &auth)
	if err == nil && status == http.StatusCreated {
		return auth, true, nil
	}
	if status != http.StatusConflict {
		return authResponse{}, false, responseError("register development account", status, err)
	}

	status, err = c.doJSON(ctx, http.MethodPost, "/auth/login", map[string]string{
		"email": cfg.Email, "password": cfg.Password,
	}, &auth)
	if err != nil || status != http.StatusOK {
		return authResponse{}, false, responseError("log in development account", status, err)
	}
	return auth, false, nil
}

func (c *apiClient) updateOrg(ctx context.Context, name, promptCompany string) error {
	status, err := c.doJSON(ctx, http.MethodPatch, "/v1/orgs/current", map[string]string{
		"name": name, "prompt_company": promptCompany,
	}, nil)
	if err != nil || status != http.StatusOK {
		return responseError("update development organization", status, err)
	}
	return nil
}

func (c *apiClient) ensureTeam(ctx context.Context, name, description string) (teamResponse, bool, error) {
	var listed struct {
		Data []teamResponse `json:"data"`
	}
	status, err := c.doJSON(ctx, http.MethodGet, "/v1/orgs/current/teams?limit=100", nil, &listed)
	if err != nil || status != http.StatusOK {
		return teamResponse{}, false, responseError("list development teams", status, err)
	}
	if len(listed.Data) > 0 {
		return listed.Data[0], false, nil
	}

	var created struct {
		Team teamResponse `json:"team"`
	}
	status, err = c.doJSON(ctx, http.MethodPost, "/v1/orgs/current/teams", map[string]string{
		"name": name, "description": description,
	}, &created)
	if err != nil || status != http.StatusCreated {
		return teamResponse{}, false, responseError("create development team", status, err)
	}
	return created.Team, true, nil
}

func responseError(action string, status int, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: unexpected HTTP status %d", action, status)
}

func (c *apiClient) doJSON(ctx context.Context, method, path string, body, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}
	if c.orgID != "" {
		req.Header.Set("X-Org-ID", c.orgID)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message := http.StatusText(resp.StatusCode)
		var apiErr struct {
			Error string `json:"error"`
		}
		if decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&apiErr); decodeErr == nil && apiErr.Error != "" {
			message = apiErr.Error
		}
		return resp.StatusCode, fmt.Errorf("Hivy API returned %d: %s", resp.StatusCode, message)
	}
	if out != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out); err != nil {
			return resp.StatusCode, fmt.Errorf("decode response: %w", err)
		}
	}
	return resp.StatusCode, nil
}
