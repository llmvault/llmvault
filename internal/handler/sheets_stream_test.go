package handler_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/sheets"
)

func (h *sheetsHarness) mintLiveToken(t *testing.T, sheetID string) string {
	t.Helper()
	resp := h.do(t, &h.org, http.MethodPost, "/v1/sheets/"+sheetID+"/live-token", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("live-token status=%d body=%s", resp.Code, resp.Body.String())
	}
	var decoded struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode live token: %v", err)
	}
	if decoded.Token == "" || decoded.ExpiresAt == "" {
		t.Fatalf("live token response = %s", resp.Body.String())
	}
	return decoded.Token
}

func TestSheetsLiveRejectsBadTokens(t *testing.T) {
	h := newSheetsHarness(t)
	rc := connectTestRedis(t)
	if err := rc.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis is not available: %v", err)
	}
	h.handler.WithRedis(rc)
	sheetID := h.sheet.Sheet.ID.String()
	livePath := "/v1/sheets/" + sheetID + "/live"

	get := func(token string, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rr := httptest.NewRecorder()
		h.router.ServeHTTP(rr, req)
		return rr
	}

	if resp := get("", livePath); resp.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status=%d", resp.Code)
	}
	if resp := get("garbage", livePath); resp.Code != http.StatusUnauthorized {
		t.Fatalf("garbage token status=%d", resp.Code)
	}

	expired := mintExpiredSheetsLiveToken(t, h.org.ID.String(), sheetID)
	if resp := get(expired, livePath); resp.Code != http.StatusUnauthorized {
		t.Fatalf("expired token status=%d", resp.Code)
	}

	wrongKey := mintSheetsLiveTokenWithKey(t, "some-other-key", h.org.ID.String(), sheetID, time.Now().Add(time.Minute))
	if resp := get(wrongKey, livePath); resp.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-key token status=%d", resp.Code)
	}

	// A valid token for sheet A must not open sheet B's stream.
	token := h.mintLiveToken(t, sheetID)
	otherLive := "/v1/sheets/" + h.otherSheet.Sheet.ID.String() + "/live"
	if resp := get(token, otherLive); resp.Code != http.StatusForbidden {
		t.Fatalf("cross-sheet token status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestSheetsLiveTokenRoundTripStreamsEvents(t *testing.T) {
	h := newSheetsHarness(t)
	rc := connectTestRedis(t)
	if err := rc.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis is not available: %v", err)
	}
	h.handler.WithRedis(rc)
	h.svc.WithPublisher(sheets.NewRedisEventPublisher(rc))

	server := httptest.NewServer(h.router)
	defer server.Close()

	token := h.mintLiveToken(t, h.sheet.Sheet.ID.String())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		server.URL+"/v1/sheets/"+h.sheet.Sheet.ID.String()+"/live", nil)
	if err != nil {
		t.Fatalf("build live request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open live stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("live status=%d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	readEvent := func() string {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("read stream: %v", err)
			}
			if strings.HasPrefix(line, "data: ") {
				return strings.TrimSpace(strings.TrimPrefix(line, "data: "))
			}
		}
	}
	ready := readEvent()
	if !strings.Contains(ready, `"ready"`) {
		t.Fatalf("first event = %s", ready)
	}

	go func() {
		// Give the SSE subscriber a beat to attach before publishing.
		time.Sleep(300 * time.Millisecond)
		_, _ = h.svc.InsertRows(
			sheets.WithMutationID(context.Background(), "live-mut-1"),
			h.org.ID, h.page.Page.ID,
			[]sheets.RowInsert{{Data: map[string]any{h.fields["Name"]: "Streamed"}}},
			sheets.MaxRowsPerWriteREST, sheets.Actor{UserID: &h.user.ID},
		)
	}()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		payload := readEvent()
		if strings.Contains(payload, `"rows_changed"`) {
			if !strings.Contains(payload, `"live-mut-1"`) {
				t.Fatalf("event missing mutation_id echo: %s", payload)
			}
			if !strings.Contains(payload, h.page.Page.ID.String()) {
				t.Fatalf("event missing page id: %s", payload)
			}
			return
		}
	}
	t.Fatalf("rows_changed event never arrived")
}

func mintExpiredSheetsLiveToken(t *testing.T, orgID, sheetID string) string {
	t.Helper()
	return mintSheetsLiveTokenWithKey(t, sheetsTestSigningKey, orgID, sheetID, time.Now().Add(-time.Minute))
}

func mintSheetsLiveTokenWithKey(t *testing.T, key, orgID, sheetID string, expiresAt time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss":      "hivy",
		"aud":      "hivy-sheets-live",
		"org_id":   orgID,
		"sheet_id": sheetID,
		"scopes":   []string{"sheets:read"},
		"iat":      time.Now().Add(-2 * time.Minute).Unix(),
		"exp":      expiresAt.Unix(),
		"jti":      uuid.NewString(),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(key))
	if err != nil {
		t.Fatalf("mint test token: %v", err)
	}
	return token
}
