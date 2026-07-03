package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestAppsInternalAuth covers the bearer gate of the internal app API: the
// app secret is the only credential, verified constant-time against the
// decrypted stored secret (authAgent pattern).
func TestAppsInternalAuth(t *testing.T) {
	h := newAppsHarness(t)

	t.Run("missing bearer is 401", func(t *testing.T) {
		resp := h.do(t, http.MethodGet, h.appPath("/sheet"), "", "", nil)
		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
		}
	})

	t.Run("wrong bearer is 401", func(t *testing.T) {
		resp := h.do(t, http.MethodGet, h.appPath("/sheet"), "hvapp_"+uuid.NewString(), "", nil)
		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
		}
	})

	t.Run("unknown app is 404", func(t *testing.T) {
		path := "/internal/apps/" + uuid.NewString() + "/v1/sheet"
		resp := h.do(t, http.MethodGet, path, h.secret, "", nil)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
		}
	})

	t.Run("malformed app id is 400", func(t *testing.T) {
		resp := h.do(t, http.MethodGet, "/internal/apps/not-a-uuid/v1/sheet", h.secret, "", nil)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
		}
	})
}

// TestAppsInternalSheetStructure verifies GET /sheet returns the bound
// sheet's pages, fields, and row counts with the correct secret.
func TestAppsInternalSheetStructure(t *testing.T) {
	h := newAppsHarness(t)

	resp := h.do(t, http.MethodPost, h.pagePath(h.page.Page.ID, "/rows"), h.secret, "", map[string]any{
		"rows": []map[string]any{{"data": map[string]any{h.fields["Title"]: "Ship apps"}}},
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("seed row status=%d body=%s", resp.Code, resp.Body.String())
	}

	resp = h.do(t, http.MethodGet, h.appPath("/sheet"), h.secret, "", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("structure status=%d body=%s", resp.Code, resp.Body.String())
	}
	var decoded struct {
		Sheet struct {
			ID string `json:"id"`
		} `json:"sheet"`
		Pages []struct {
			ID     string `json:"id"`
			Fields []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"fields"`
			RowCount int64 `json:"row_count"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode structure: %v", err)
	}
	if decoded.Sheet.ID != h.sheet.Sheet.ID.String() {
		t.Fatalf("structure returned sheet %s, want bound sheet %s", decoded.Sheet.ID, h.sheet.Sheet.ID)
	}
	if len(decoded.Pages) != 1 || decoded.Pages[0].ID != h.page.Page.ID.String() {
		t.Fatalf("unexpected pages: %+v", decoded.Pages)
	}
	if len(decoded.Pages[0].Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(decoded.Pages[0].Fields))
	}
	if decoded.Pages[0].RowCount != 1 {
		t.Fatalf("expected row_count 1, got %d", decoded.Pages[0].RowCount)
	}
}

// TestAppsInternalPageOutsideBoundSheet proves the one-sheet blast radius: a
// page of another sheet in the SAME org and channel is a 404 on every route.
func TestAppsInternalPageOutsideBoundSheet(t *testing.T) {
	h := newAppsHarness(t)
	foreign := h.unboundPage.Page.ID

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"query", http.MethodPost, h.pagePath(foreign, "/rows/query"), map[string]any{}},
		{"insert", http.MethodPost, h.pagePath(foreign, "/rows"), map[string]any{
			"rows": []map[string]any{{"data": map[string]any{"fld_x": "v"}}},
		}},
		{"update", http.MethodPatch, h.pagePath(foreign, "/rows"), map[string]any{
			"rows": []map[string]any{{"id": uuid.NewString(), "data": map[string]any{"fld_x": "v"}}},
		}},
		{"delete", http.MethodDelete, h.pagePath(foreign, "/rows"), map[string]any{
			"ids": []string{uuid.NewString()},
		}},
		{"attachments", http.MethodPost, h.pagePath(foreign, "/attachments/download-url"), map[string]any{
			"keys": []string{"pub/o/whatever"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := h.do(t, tc.method, tc.path, h.secret, "", tc.body)
			if resp.Code != http.StatusNotFound {
				t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
			}
		})
	}

	// The unbound sheet stays untouched and reachable through its own scope
	// (sanity that the 404 is about app binding, not a broken page).
	var count int64
	if err := h.db.Table("sheet_rows").Where("page_id = ?", foreign).Count(&count).Error; err != nil {
		t.Fatalf("count unbound rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("unbound page gained %d rows through the app API", count)
	}
}
