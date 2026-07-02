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
	ownKey := "pub/o/" + h.org.ID.String() + "/sheets/attachments/file.png"
	resp := h.do(t, &h.org, http.MethodPost, h.pagePath("/attachments/download-url"), map[string]any{
		"keys": []string{ownKey},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("download-url status=%d body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "https://storage.test/"+ownKey) {
		t.Fatalf("missing presigned url: %s", resp.Body.String())
	}

	foreignKey := "pub/o/" + h.other.ID.String() + "/sheets/attachments/secret.png"
	resp = h.do(t, &h.org, http.MethodPost, h.pagePath("/attachments/download-url"), map[string]any{
		"keys": []string{foreignKey},
	})
	if resp.Code != http.StatusForbidden {
		t.Fatalf("foreign key status=%d body=%s", resp.Code, resp.Body.String())
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
}
