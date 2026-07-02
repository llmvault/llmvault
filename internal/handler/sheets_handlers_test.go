package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/sheets"
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
	// The harness already created one sheet; add two more so the org has 3.
	for _, name := range []string{"Cursor B", "Cursor C"} {
		resp := h.do(t, &h.org, http.MethodPost, "/v1/sheets", map[string]any{"name": name})
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
	fetch := func(query string) listPage {
		t.Helper()
		resp := h.do(t, &h.org, http.MethodGet, "/v1/sheets"+query, nil)
		if resp.Code != http.StatusOK {
			t.Fatalf("list %q status=%d body=%s", query, resp.Code, resp.Body.String())
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
		query := "?limit=2"
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
	resp := h.do(t, &h.org, http.MethodGet, "/v1/sheets?cursor=not-a-cursor", nil)
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

func TestSheetsOperationsRevertEndpoint(t *testing.T) {
	h := newSheetsHarness(t)
	nameID := h.fields["Name"]
	h.insertRows(t, map[string]any{nameID: "Undo me"})

	list := h.do(t, &h.org, http.MethodGet, h.pagePath("/operations"), nil)
	if list.Code != http.StatusOK {
		t.Fatalf("operations status=%d body=%s", list.Code, list.Body.String())
	}
	var ops struct {
		Operations []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &ops); err != nil {
		t.Fatalf("decode operations: %v", err)
	}
	if len(ops.Operations) == 0 || ops.Operations[0].Type != "rows_insert" {
		t.Fatalf("operations = %+v", ops.Operations)
	}
	opID := ops.Operations[0].ID

	if resp := h.do(t, &h.other, http.MethodPost, h.pagePath("/operations/"+opID+"/revert"), nil); resp.Code != http.StatusNotFound {
		t.Fatalf("cross-org revert status=%d, want 404", resp.Code)
	}
	if resp := h.do(t, &h.org, http.MethodPost, h.pagePath("/operations/"+opID+"/revert"), nil); resp.Code != http.StatusOK {
		t.Fatalf("revert status=%d body=%s", resp.Code, resp.Body.String())
	}
	query := h.do(t, &h.org, http.MethodPost, h.pagePath("/rows/query"), map[string]any{})
	if strings.Contains(query.Body.String(), "Undo me") {
		t.Fatalf("reverted insert still visible: %s", query.Body.String())
	}
	if resp := h.do(t, &h.org, http.MethodPost, h.pagePath("/operations/"+opID+"/revert"), nil); resp.Code != http.StatusConflict {
		t.Fatalf("double revert status=%d, want 409", resp.Code)
	}
}

func TestSheetsAttachmentDownloadURL(t *testing.T) {
	h := newSheetsHarness(t)
	orgAgent := h.createAgent(t, h.org.ID)
	foreignAgent := h.createAgent(t, h.other.ID)

	// Accepted: any org-owned pub/o/{orgID}/ key, plus drive keys of the
	// org's OWN agents — the same admission rule cell validation applies
	// (sheets.Service.AuthorizeObjectKeys), so whatever a cell can hold,
	// download-url can sign.
	ownKey := "pub/o/" + h.org.ID.String() + "/sheets/attachments/file.png"
	brandKey := "pub/o/" + h.org.ID.String() + "/brand-assets/logo.png"
	driveKey := "pub/e/" + orgAgent.ID.String() + "/report.pdf"
	resp := h.do(t, &h.org, http.MethodPost, h.pagePath("/attachments/download-url"), map[string]any{
		"keys": []string{ownKey, brandKey, driveKey},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("download-url status=%d body=%s", resp.Code, resp.Body.String())
	}
	for _, key := range []string{ownKey, brandKey, driveKey} {
		if !strings.Contains(resp.Body.String(), "https://storage.test/"+key) {
			t.Fatalf("missing presigned url for %s: %s", key, resp.Body.String())
		}
	}

	// Rejected: foreign-org keys, drive keys of a foreign org's agent or a
	// non-existent agent, traversal.
	rejected := []string{
		"pub/o/" + h.other.ID.String() + "/sheets/attachments/secret.png",
		"pub/e/" + foreignAgent.ID.String() + "/report.pdf",
		"pub/e/" + h.user.ID.String() + "/report.pdf", // no agent with this ID
		"pub/e/" + orgAgent.ID.String() + "/../escape.pdf",
		"pub/o/" + h.org.ID.String() + "/sheets/attachments/../../escape.png",
	}
	for _, key := range rejected {
		resp = h.do(t, &h.org, http.MethodPost, h.pagePath("/attachments/download-url"), map[string]any{
			"keys": []string{key},
		})
		if resp.Code != http.StatusForbidden {
			t.Fatalf("key %q status=%d body=%s, want 403", key, resp.Code, resp.Body.String())
		}
	}
}

func TestSheetsImportEndpoints(t *testing.T) {
	h := newSheetsHarness(t)
	key := "pub/o/" + h.org.ID.String() + "/sheetimports/leads.csv"
	created := h.do(t, &h.org, http.MethodPost, h.pagePath("/imports"), map[string]any{"object_key": key})
	if created.Code != http.StatusCreated {
		t.Fatalf("create import status=%d body=%s", created.Code, created.Body.String())
	}
	var job struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &job); err != nil {
		t.Fatalf("decode import job: %v", err)
	}
	if job.Status != "pending" {
		t.Fatalf("job status = %q", job.Status)
	}

	status := h.do(t, &h.org, http.MethodGet, "/v1/sheets/imports/"+job.ID, nil)
	if status.Code != http.StatusOK {
		t.Fatalf("import status=%d body=%s", status.Code, status.Body.String())
	}
	if resp := h.do(t, &h.other, http.MethodGet, "/v1/sheets/imports/"+job.ID, nil); resp.Code != http.StatusNotFound {
		t.Fatalf("cross-org import status=%d, want 404", resp.Code)
	}

	foreign := h.do(t, &h.org, http.MethodPost, h.pagePath("/imports"), map[string]any{
		"object_key": "pub/o/" + h.other.ID.String() + "/sheetimports/steal.csv",
	})
	if foreign.Code != http.StatusBadRequest {
		t.Fatalf("foreign import key status=%d body=%s", foreign.Code, foreign.Body.String())
	}

	// Agent drive keys: accepted for the org's own agent (the SKILL.md drive
	// upload → import flow), rejected for a foreign org's agent.
	orgAgent := h.createAgent(t, h.org.ID)
	foreignAgent := h.createAgent(t, h.other.ID)
	drive := h.do(t, &h.org, http.MethodPost, h.pagePath("/imports"), map[string]any{
		"object_key": "pub/e/" + orgAgent.ID.String() + "/imports/leads.csv",
	})
	if drive.Code != http.StatusCreated {
		t.Fatalf("org-agent drive import status=%d body=%s, want 201", drive.Code, drive.Body.String())
	}
	foreignDrive := h.do(t, &h.org, http.MethodPost, h.pagePath("/imports"), map[string]any{
		"object_key": "pub/e/" + foreignAgent.ID.String() + "/imports/leads.csv",
	})
	if foreignDrive.Code != http.StatusBadRequest {
		t.Fatalf("foreign-agent drive import status=%d body=%s, want 400", foreignDrive.Code, foreignDrive.Body.String())
	}
}

// TestSheetsNestedRoutesRejectWrongSheet pins the §2b nested-route contract:
// a page addressed under a different same-org sheet's ID is a 404, even
// though both resources belong to the caller's org.
func TestSheetsNestedRoutesRejectWrongSheet(t *testing.T) {
	h := newSheetsHarness(t)
	rowID := h.insertRows(t, map[string]any{h.fields["Name"]: "keep me"})[0]

	// A second sheet in the SAME org; its ID must not grant access to h.page.
	second, err := h.svc.CreateSheet(context.Background(), h.org.ID, sheets.CreateSheetRequest{
		Name:  "Second Sheet",
		Pages: []sheets.PageSpec{{Name: "Other Page"}},
	}, sheets.Actor{UserID: &h.user.ID})
	if err != nil {
		t.Fatalf("create second sheet: %v", err)
	}
	wrongBase := "/v1/sheets/" + second.Sheet.ID.String() + "/pages/" + h.page.Page.ID.String()

	cases := []struct {
		name, method, path string
		body               any
	}{
		{"rows query", http.MethodPost, wrongBase + "/rows/query", map[string]any{}},
		{"rows patch", http.MethodPatch, wrongBase + "/rows", map[string]any{
			"rows": []map[string]any{{"id": rowID, "data": map[string]any{h.fields["Name"]: "stolen"}}},
		}},
		{"operations list", http.MethodGet, wrongBase + "/operations", nil},
		{"imports create", http.MethodPost, wrongBase + "/imports", map[string]any{
			"object_key": "pub/o/" + h.org.ID.String() + "/sheetimports/x.csv",
		}},
	}
	for _, tc := range cases {
		resp := h.do(t, &h.org, tc.method, tc.path, tc.body)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("%s under wrong sheet: status=%d body=%s, want 404", tc.name, resp.Code, resp.Body.String())
		}
	}

	// The same operations under the correct sheet ID still work.
	if resp := h.do(t, &h.org, http.MethodPost, h.pagePath("/rows/query"), map[string]any{}); resp.Code != http.StatusOK {
		t.Fatalf("rows query under correct sheet: status=%d body=%s", resp.Code, resp.Body.String())
	}
	if resp := h.do(t, &h.org, http.MethodGet, h.pagePath("/operations"), nil); resp.Code != http.StatusOK {
		t.Fatalf("operations under correct sheet: status=%d body=%s", resp.Code, resp.Body.String())
	}
}
