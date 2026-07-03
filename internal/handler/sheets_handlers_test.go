package handler_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSheetsOrgIsolation(t *testing.T) {
	h := newSheetsHarness(t)
	otherSheetID := h.otherSheet.Sheet.ID.String()
	otherPagePath := "/v1/sheets/" + otherSheetID + "/pages/" + h.otherPage.Page.ID.String()

	cases := []struct {
		name, method, path string
		body               any
	}{
		{"get sheet", http.MethodGet, "/v1/sheets/" + otherSheetID, nil},
		{"patch sheet", http.MethodPatch, "/v1/sheets/" + otherSheetID, map[string]any{"name": "stolen"}},
		{"archive sheet", http.MethodDelete, "/v1/sheets/" + otherSheetID, nil},
		{"rows query", http.MethodPost, otherPagePath + "/rows/query", map[string]any{}},
		{"rows insert", http.MethodPost, otherPagePath + "/rows", map[string]any{
			"rows": []map[string]any{{"data": map[string]any{}}},
		}},
		{"export", http.MethodGet, otherPagePath + "/export.csv", nil},
		{"operations", http.MethodGet, otherPagePath + "/operations", nil},
		{"live token", http.MethodPost, "/v1/sheets/" + otherSheetID + "/live-token", nil},
	}
	for _, tc := range cases {
		resp := h.do(t, &h.org, tc.method, tc.path, tc.body)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("%s: status=%d body=%s, want 404", tc.name, resp.Code, resp.Body.String())
		}
	}

	// The org's own sheet stays reachable.
	resp := h.do(t, &h.org, http.MethodGet, "/v1/sheets/"+h.sheet.Sheet.ID.String(), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("own sheet status=%d body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"row_count"`) {
		t.Fatalf("structure response missing row counts: %s", resp.Body.String())
	}
}

func TestSheetsListCursorPagination(t *testing.T) {
	h := newSheetsHarness(t)
	channelID := h.channel.ID.String()
	// The harness already created one sheet; add two more so the channel has 3.
	for _, name := range []string{"Cursor B", "Cursor C"} {
		resp := h.do(t, &h.org, http.MethodPost, "/v1/sheets", map[string]any{"name": name, "channel_id": channelID})
		if resp.Code != http.StatusCreated {
			t.Fatalf("create sheet %q status=%d body=%s", name, resp.Code, resp.Body.String())
		}
	}

	type listPage struct {
		Sheets []struct {
			ID string `json:"id"`
		} `json:"sheets"`
		NextCursor string `json:"next_cursor"`
	}
	fetch := func(extra string) listPage {
		t.Helper()
		resp := h.do(t, &h.org, http.MethodGet, "/v1/sheets?channel_id="+channelID+extra, nil)
		if resp.Code != http.StatusOK {
			t.Fatalf("list %q status=%d body=%s", extra, resp.Code, resp.Body.String())
		}
		var page listPage
		if err := json.Unmarshal(resp.Body.Bytes(), &page); err != nil {
			t.Fatalf("decode list page: %v", err)
		}
		return page
	}

	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		query := "&limit=2"
		if cursor != "" {
			query += "&cursor=" + cursor
		}
		page := fetch(query)
		if len(page.Sheets) == 0 {
			t.Fatalf("page %d returned no sheets", pages)
		}
		for _, sheet := range page.Sheets {
			if seen[sheet.ID] {
				t.Fatalf("cursor walk repeated sheet %s", sheet.ID)
			}
			seen[sheet.ID] = true
		}
		pages++
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
		if pages > 5 {
			t.Fatalf("cursor walk did not terminate")
		}
	}
	if len(seen) != 3 || pages != 2 {
		t.Fatalf("cursor walk saw %d sheets over %d pages, want 3 over 2", len(seen), pages)
	}

	// A full first page (no cursor) still reports next_cursor empty when the
	// org fits in one page — backward-compatible shape.
	full := fetch("")
	if len(full.Sheets) != 3 || full.NextCursor != "" {
		t.Fatalf("full list = %d sheets next_cursor=%q", len(full.Sheets), full.NextCursor)
	}

	// Garbage cursors are a 400, not a 500.
	resp := h.do(t, &h.org, http.MethodGet, "/v1/sheets?channel_id="+channelID+"&cursor=not-a-cursor", nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("bad cursor status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestSheetsRowsPatchPartialMerge(t *testing.T) {
	h := newSheetsHarness(t)
	nameID, scoreID, mailID := h.fields["Name"], h.fields["Score"], h.fields["Mail"]
	ids := h.insertRows(t, map[string]any{nameID: "Acme", scoreID: 10, mailID: "a@acme.com"})

	resp := h.do(t, &h.org, http.MethodPatch, h.pagePath("/rows"), map[string]any{
		"rows":        []map[string]any{{"id": ids[0], "data": map[string]any{scoreID: 99, mailID: nil}}},
		"mutation_id": "cell-edit-1",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", resp.Code, resp.Body.String())
	}
	var decoded struct {
		Rows []struct {
			Data map[string]any `json:"data"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode patch response: %v", err)
	}
	data := decoded.Rows[0].Data
	if data[nameID] != "Acme" {
		t.Fatalf("untouched key clobbered: %v", data)
	}
	if data[scoreID] != float64(99) {
		t.Fatalf("patched key not applied: %v", data)
	}
	if _, present := data[mailID]; present {
		t.Fatalf("nil value should clear the cell: %v", data)
	}

	// Unknown field IDs are rejected, not silently stored.
	resp = h.do(t, &h.org, http.MethodPatch, h.pagePath("/rows"), map[string]any{
		"rows": []map[string]any{{"id": ids[0], "data": map[string]any{"fld_0000000000": "x"}}},
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestSheetsExportCSVStreams(t *testing.T) {
	h := newSheetsHarness(t)
	nameID, scoreID := h.fields["Name"], h.fields["Score"]
	h.insertRows(t,
		map[string]any{nameID: "One", scoreID: 1},
		map[string]any{nameID: "Two", scoreID: 2},
		map[string]any{nameID: "Comma, Inc", scoreID: 3},
	)
	resp := h.do(t, &h.org, http.MethodGet, h.pagePath("/export.csv"), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("export status=%d body=%s", resp.Code, resp.Body.String())
	}
	if ct := resp.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("content type = %q", ct)
	}
	lines := strings.Split(strings.TrimSpace(resp.Body.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("csv lines = %d body=%s", len(lines), resp.Body.String())
	}
	if !strings.Contains(lines[0], "Name") || !strings.Contains(lines[0], "Score") {
		t.Fatalf("csv header = %q", lines[0])
	}
	if !strings.Contains(resp.Body.String(), `"Comma, Inc"`) {
		t.Fatalf("csv quoting missing: %s", resp.Body.String())
	}
}
