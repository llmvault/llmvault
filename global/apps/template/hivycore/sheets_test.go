package hivycore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type capturedRequest struct {
	method string
	path   string
	bearer string
	actor  string
	body   []byte
}

// newSheetsTestServer records every request and replies with the canned
// status + JSON body.
func newSheetsTestServer(t *testing.T, status int, response string) (*SheetsClient, *capturedRequest) {
	t.Helper()
	captured := &capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.method = r.Method
		captured.path = r.URL.Path
		captured.bearer = r.Header.Get("Authorization")
		captured.actor = r.Header.Get("X-Hivy-App-Actor")
		captured.body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)
	return newSheetsClient(server.URL, "hvapp_testsecret"), captured
}

func actorContext(userID string) context.Context {
	return withSession(context.Background(), &Session{
		UserID:    userID,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
}

const testUserID = "6a4f0e0a-1111-4f4f-8888-aaaaaaaaaaaa"

func TestSheetsStructure(t *testing.T) {
	client, captured := newSheetsTestServer(t, http.StatusOK,
		`{"sheet":{"id":"s1","name":"Leads"},"pages":[{"id":"p1","name":"Main","row_count":42,"fields":[{"id":"fld_a","name":"Company","type":"text"}]}]}`)

	structure, err := client.Structure(context.Background())
	if err != nil {
		t.Fatalf("Structure: %v", err)
	}
	if captured.method != http.MethodGet || captured.path != "/v1/sheet" {
		t.Fatalf("request = %s %s, want GET /v1/sheet", captured.method, captured.path)
	}
	if captured.bearer != "Bearer hvapp_testsecret" {
		t.Fatalf("bearer = %q", captured.bearer)
	}
	if captured.actor != "" {
		t.Fatalf("read call sent actor header %q", captured.actor)
	}
	if structure.Sheet.Name != "Leads" || len(structure.Pages) != 1 || structure.Pages[0].RowCount != 42 {
		t.Fatalf("structure decoded wrong: %+v", structure)
	}
	if structure.Pages[0].Fields[0].ID != "fld_a" {
		t.Fatalf("fields decoded wrong: %+v", structure.Pages[0].Fields)
	}
}

func TestSheetsQueryRows(t *testing.T) {
	client, captured := newSheetsTestServer(t, http.StatusOK,
		`{"rows":[{"id":"r1","data":{"fld_a":"Acme"},"position":1}],"fields":[{"id":"fld_a","name":"Company","type":"text"}],"next_cursor":"abc","relations":{}}`)

	query := Query{
		Filter: &Filter{And: []Filter{{Field: "fld_a", Op: "eq", Value: "Acme"}}},
		Sorts:  []Sort{{Field: "created_at", Desc: true}},
		Limit:  10,
	}
	result, err := client.QueryRows(context.Background(), "p1", query)
	if err != nil {
		t.Fatalf("QueryRows: %v", err)
	}
	if captured.method != http.MethodPost || captured.path != "/v1/pages/p1/rows/query" {
		t.Fatalf("request = %s %s", captured.method, captured.path)
	}
	var sent map[string]any
	if err := json.Unmarshal(captured.body, &sent); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	filter := sent["filter"].(map[string]any)
	if filter["and"].([]any)[0].(map[string]any)["field"] != "fld_a" {
		t.Fatalf("filter AST not sent: %s", captured.body)
	}
	if sent["limit"].(float64) != 10 {
		t.Fatalf("limit not sent: %s", captured.body)
	}
	if result.NextCursor != "abc" || result.Rows[0].Data["fld_a"] != "Acme" {
		t.Fatalf("result decoded wrong: %+v", result)
	}
}

func TestSheetsInsertRowsSendsActor(t *testing.T) {
	client, captured := newSheetsTestServer(t, http.StatusCreated,
		`{"rows":[{"id":"r1","data":{"fld_a":"Acme"}}]}`)

	rows, err := client.InsertRows(actorContext(testUserID), "p1", []RowInsert{
		{Data: map[string]any{"fld_a": "Acme"}},
	})
	if err != nil {
		t.Fatalf("InsertRows: %v", err)
	}
	if captured.method != http.MethodPost || captured.path != "/v1/pages/p1/rows" {
		t.Fatalf("request = %s %s", captured.method, captured.path)
	}
	if captured.actor != testUserID {
		t.Fatalf("actor header = %q, want session user", captured.actor)
	}
	if captured.bearer != "Bearer hvapp_testsecret" {
		t.Fatalf("bearer = %q", captured.bearer)
	}
	var sent struct {
		Rows []RowInsert `json:"rows"`
	}
	if err := json.Unmarshal(captured.body, &sent); err != nil || len(sent.Rows) != 1 || sent.Rows[0].Data["fld_a"] != "Acme" {
		t.Fatalf("insert body wrong: %s (err %v)", captured.body, err)
	}
	if len(rows) != 1 || rows[0].ID != "r1" {
		t.Fatalf("rows decoded wrong: %+v", rows)
	}
}

func TestSheetsUpdateRows(t *testing.T) {
	client, captured := newSheetsTestServer(t, http.StatusOK,
		`{"rows":[{"id":"r1","data":{"fld_a":"Updated"}}]}`)

	_, err := client.UpdateRows(actorContext(testUserID), "p1", []RowUpdate{
		{ID: "r1", Data: map[string]any{"fld_a": "Updated"}},
	})
	if err != nil {
		t.Fatalf("UpdateRows: %v", err)
	}
	if captured.method != http.MethodPatch || captured.path != "/v1/pages/p1/rows" {
		t.Fatalf("request = %s %s, want PATCH /v1/pages/p1/rows", captured.method, captured.path)
	}
	if captured.actor != testUserID {
		t.Fatalf("actor header = %q", captured.actor)
	}
}

func TestSheetsDeleteRows(t *testing.T) {
	client, captured := newSheetsTestServer(t, http.StatusOK, `{"archived":2}`)

	archived, err := client.DeleteRows(actorContext(testUserID), "p1", []string{"r1", "r2"})
	if err != nil {
		t.Fatalf("DeleteRows: %v", err)
	}
	if captured.method != http.MethodDelete || captured.path != "/v1/pages/p1/rows" {
		t.Fatalf("request = %s %s, want DELETE /v1/pages/p1/rows", captured.method, captured.path)
	}
	if captured.actor != testUserID {
		t.Fatalf("actor header = %q", captured.actor)
	}
	var sent struct {
		IDs []string `json:"ids"`
	}
	if err := json.Unmarshal(captured.body, &sent); err != nil || len(sent.IDs) != 2 {
		t.Fatalf("delete body wrong: %s", captured.body)
	}
	if archived != 2 {
		t.Fatalf("archived = %d, want 2", archived)
	}
}

func TestSheetsAttachmentDownloadURLs(t *testing.T) {
	client, captured := newSheetsTestServer(t, http.StatusOK,
		`{"urls":{"pub/o/org/k1":"https://signed.example/k1"}}`)

	urls, err := client.AttachmentDownloadURLs(context.Background(), "p1", []string{"pub/o/org/k1"})
	if err != nil {
		t.Fatalf("AttachmentDownloadURLs: %v", err)
	}
	if captured.method != http.MethodPost || captured.path != "/v1/pages/p1/attachments/download-url" {
		t.Fatalf("request = %s %s", captured.method, captured.path)
	}
	if urls["pub/o/org/k1"] != "https://signed.example/k1" {
		t.Fatalf("urls decoded wrong: %+v", urls)
	}
}

func TestSheetsAPIErrorSurfaced(t *testing.T) {
	client, _ := newSheetsTestServer(t, http.StatusUnprocessableEntity,
		`{"error":"sheets: batch exceeds max of 100 rows"}`)

	_, err := client.InsertRows(actorContext(testUserID), "p1", []RowInsert{{Data: map[string]any{}}})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is %T (%v), want *APIError", err, err)
	}
	if apiErr.Status != http.StatusUnprocessableEntity || apiErr.Message != "sheets: batch exceeds max of 100 rows" {
		t.Fatalf("APIError = %+v", apiErr)
	}
}

func TestSheetsEmptyBatchesRejectedClientSide(t *testing.T) {
	client, captured := newSheetsTestServer(t, http.StatusOK, `{}`)
	ctx := context.Background()
	if _, err := client.InsertRows(ctx, "p1", nil); err == nil {
		t.Fatal("empty insert accepted")
	}
	if _, err := client.UpdateRows(ctx, "p1", nil); err == nil {
		t.Fatal("empty update accepted")
	}
	if _, err := client.DeleteRows(ctx, "p1", nil); err == nil {
		t.Fatal("empty delete accepted")
	}
	if _, err := client.AttachmentDownloadURLs(ctx, "p1", nil); err == nil {
		t.Fatal("empty keys accepted")
	}
	if _, err := client.QueryRows(ctx, "", Query{}); err == nil {
		t.Fatal("empty pageID accepted")
	}
	if captured.method != "" {
		t.Fatalf("client-side validation still hit the server: %s %s", captured.method, captured.path)
	}
}
