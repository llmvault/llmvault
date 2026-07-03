package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/sheets"
)

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
