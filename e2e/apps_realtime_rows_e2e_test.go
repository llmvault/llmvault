package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// appsRealtimeSheet captures the identifiers the realtime test needs after a
// POST /v1/sheets: the sheet ID, its single page ID, and the field IDs keyed by
// field name (row data is keyed by field ID, never by display name).
type appsRealtimeSheet struct {
	SheetID  string
	PageID   string
	FieldIDs map[string]string
}

// appsRealtimeCreateSheet creates a sheet with one page and the named text
// fields, returning the resolved IDs.
func appsRealtimeCreateSheet(t *testing.T, ctx context.Context, apiBase, token, orgID, channelID, name string, fields []string) appsRealtimeSheet {
	t.Helper()
	fieldSpecs := make([]map[string]any, 0, len(fields))
	for _, f := range fields {
		fieldSpecs = append(fieldSpecs, map[string]any{"name": f, "type": "text"})
	}
	var out struct {
		Sheet struct {
			ID string `json:"id"`
		} `json:"sheet"`
		Pages []struct {
			ID     string `json:"id"`
			Fields []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"fields"`
		} `json:"pages"`
	}
	agentSessionsJSON(t, ctx, http.MethodPost, apiBase+"/v1/sheets", token, orgID, map[string]any{
		"channel_id": channelID,
		"name":       name,
		"pages": []map[string]any{{
			"name":   "Main",
			"fields": fieldSpecs,
		}},
	}, http.StatusCreated, &out)
	if out.Sheet.ID == "" || len(out.Pages) != 1 || out.Pages[0].ID == "" {
		t.Fatalf("create sheet %q returned unexpected structure: %+v", name, out)
	}
	ids := map[string]string{}
	for _, f := range out.Pages[0].Fields {
		ids[f.Name] = f.ID
	}
	for _, want := range fields {
		if ids[want] == "" {
			t.Fatalf("sheet %q missing field id for %q: %+v", name, want, ids)
		}
	}
	return appsRealtimeSheet{SheetID: out.Sheet.ID, PageID: out.Pages[0].ID, FieldIDs: ids}
}

// appsRealtimeRowResult is the {rows:[...]} shape returned by insert/update.
type appsRealtimeRowResult struct {
	Rows []struct {
		ID   string         `json:"id"`
		Data map[string]any `json:"data"`
	} `json:"rows"`
}

// appsRealtimeInsertRows inserts rows into a page via the sheets REST API and
// returns the created row IDs (in order).
func appsRealtimeInsertRows(t *testing.T, ctx context.Context, apiBase, token, orgID string, s appsRealtimeSheet, rows []map[string]any) []string {
	t.Helper()
	payloadRows := make([]map[string]any, 0, len(rows))
	for _, data := range rows {
		payloadRows = append(payloadRows, map[string]any{"data": data})
	}
	var out appsRealtimeRowResult
	agentSessionsJSON(t, ctx, http.MethodPost,
		apiBase+"/v1/sheets/"+s.SheetID+"/pages/"+s.PageID+"/rows",
		token, orgID, map[string]any{"rows": payloadRows}, http.StatusCreated, &out)
	if len(out.Rows) != len(rows) {
		t.Fatalf("insert returned %d rows want %d", len(out.Rows), len(rows))
	}
	ids := make([]string, 0, len(out.Rows))
	for _, r := range out.Rows {
		ids = append(ids, r.ID)
	}
	return ids
}

// appsRealtimeUpdateRow partial-merges data into a single row via REST.
func appsRealtimeUpdateRow(t *testing.T, ctx context.Context, apiBase, token, orgID string, s appsRealtimeSheet, rowID string, data map[string]any) {
	t.Helper()
	var out appsRealtimeRowResult
	agentSessionsJSON(t, ctx, http.MethodPatch,
		apiBase+"/v1/sheets/"+s.SheetID+"/pages/"+s.PageID+"/rows",
		token, orgID, map[string]any{"rows": []map[string]any{{"id": rowID, "data": data}}}, http.StatusOK, &out)
	if len(out.Rows) != 1 {
		t.Fatalf("update returned %d rows want 1", len(out.Rows))
	}
}

// appsRealtimeDeleteRow archives a single row via REST.
func appsRealtimeDeleteRow(t *testing.T, ctx context.Context, apiBase, token, orgID string, s appsRealtimeSheet, rowID string) {
	t.Helper()
	var out struct {
		Archived int64 `json:"archived"`
	}
	agentSessionsJSON(t, ctx, http.MethodDelete,
		apiBase+"/v1/sheets/"+s.SheetID+"/pages/"+s.PageID+"/rows",
		token, orgID, map[string]any{"ids": []string{rowID}}, http.StatusOK, &out)
	if out.Archived != 1 {
		t.Fatalf("delete archived=%d want 1", out.Archived)
	}
}

// appsRealtimeQueryAppRows queries the DEPLOYED app's own read path
// (POST /api/pages/{pageID}/rows/query with the session cookie) — the same
// endpoint the SPA's useRows hook calls — and returns the rows keyed by ID.
func appsRealtimeQueryAppRows(t *testing.T, ctx context.Context, appBase, cookie, pageID string) map[string]map[string]any {
	t.Helper()
	resp := appsFlagshipDo(t, ctx, http.MethodPost, appBase+"/api/pages/"+pageID+"/rows/query",
		map[string]string{"Content-Type": "application/json", "Cookie": "hivy_app_session=" + cookie}, []byte(`{}`))
	if resp.Status != http.StatusOK {
		t.Fatalf("app rows/query status=%d body=%s", resp.Status, resp.Body)
	}
	var out struct {
		Rows []struct {
			ID   string         `json:"id"`
			Data map[string]any `json:"data"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatalf("decode app rows/query: %v body=%s", err, resp.Body)
	}
	byID := map[string]map[string]any{}
	for _, r := range out.Rows {
		byID[r.ID] = r.Data
	}
	return byID
}
