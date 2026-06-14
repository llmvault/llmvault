package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

func agentSessionsBaseURL(explicitEnv, portEnv, fallbackPort string) string {
	if value := strings.TrimRight(strings.TrimSpace(os.Getenv(explicitEnv)), "/"); value != "" {
		return value
	}
	port := strings.TrimSpace(os.Getenv(portEnv))
	if port == "" {
		port = fallbackPort
	}
	return "http://localhost:" + port
}

func requireAgentSessionsHealthy(t *testing.T, ctx context.Context, baseURL, service string) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
	if err != nil {
		t.Fatalf("build %s health request: %v", service, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s at %s is not reachable; start the compose stack first: %v", service, baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s health status=%d body=%s", service, resp.StatusCode, body)
	}
}

func agentSessionsRegister(t *testing.T, ctx context.Context, baseURL, email, password, name string) authResponseDTO {
	t.Helper()
	body := map[string]string{"email": email, "password": password, "name": name}
	var out authResponseDTO
	agentSessionsJSON(t, ctx, http.MethodPost, baseURL+"/auth/register", "", "", body, http.StatusCreated, &out)
	if out.AccessToken == "" {
		t.Fatalf("register returned empty access token")
	}
	return out
}

func agentSessionsLogin(t *testing.T, ctx context.Context, baseURL, email, password, orgID string) authResponseDTO {
	t.Helper()
	body := map[string]string{"email": email, "password": password, "org_id": orgID}
	var out authResponseDTO
	agentSessionsJSON(t, ctx, http.MethodPost, baseURL+"/auth/login", "", "", body, http.StatusOK, &out)
	if out.AccessToken == "" {
		t.Fatalf("login returned empty access token")
	}
	return out
}

func agentSessionsAddOrgMemberFixture(t *testing.T, orgIDRaw, userIDRaw, role string) {
	t.Helper()
	db := agentSessionsOpenDB(t)
	orgID := uuid.MustParse(orgIDRaw)
	userID := uuid.MustParse(userIDRaw)
	m := model.OrgMembership{OrgID: orgID, UserID: userID, Role: role}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "org_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"role"}),
	}).Create(&m).Error; err != nil {
		t.Fatalf("add org membership fixture: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ? AND user_id = ?", orgID, userID).Delete(&model.OrgMembership{})
	})
}

func agentSessionsOpenDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(agentSessionsDatabaseURL()), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect e2e database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("unwrap e2e database: %v", err)
	}
	sqlDB.SetMaxOpenConns(2)
	sqlDB.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func agentSessionsDatabaseURL() string {
	if value := strings.TrimSpace(os.Getenv("AGENT_SESSIONS_E2E_DATABASE_URL")); value != "" {
		return value
	}
	port := strings.TrimSpace(os.Getenv("HIVY_COMPOSE_POSTGRES_PORT"))
	if port == "" {
		port = "5433"
	}
	return "postgres://hivy:localdev@localhost:" + port + "/hivy?sslmode=disable"
}

func agentSessionsJSON(t *testing.T, ctx context.Context, method, url, token, orgID string, body any, wantStatus int, out any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal %s %s: %v", method, url, err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, url, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if orgID != "" {
		req.Header.Set("X-Org-ID", orgID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, url, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, url, resp.StatusCode, wantStatus, raw)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("decode %s %s: %v\n%s", method, url, err, raw)
		}
	}
}

func waitForAgentSessionsResponse(t *testing.T, ctx context.Context, baseURL, token, orgID, sessionID, marker string) agentSessionsEvent {
	t.Helper()
	deadline := time.Now().Add(8 * time.Minute)
	var lastTypes []string
	for time.Now().Before(deadline) {
		events := agentSessionsListEvents(t, ctx, baseURL, token, orgID, sessionID)
		lastTypes = lastTypes[:0]
		for _, event := range events {
			lastTypes = append(lastTypes, event.EventType)
			raw, _ := json.Marshal(event.Payload)
			if event.EventType == "agent.message.sent" && strings.Contains(string(raw), marker) {
				return event
			}
		}
		t.Logf("waiting for agent response marker=%s event_types=%v", marker, lastTypes)
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for agent response: %v", ctx.Err())
		case <-time.After(5 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for agent response marker=%s last_event_types=%v", marker, lastTypes)
	return agentSessionsEvent{}
}
