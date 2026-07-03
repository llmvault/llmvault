// DO NOT EDIT — hivycore is the Hivy platform core. See doc.go.

package hivycore

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

// actorHeader carries the acting user's ID on mutating calls so the platform
// records the mutation against that user on the sheet's operation log.
const actorHeader = "X-Hivy-App-Actor"

const (
	sheetsRequestTimeout = 30 * time.Second
	maxErrorBodyBytes    = 4 << 20
)

// SheetsClient is the only data path a Hivy app has: a typed client over the
// internal app API, authenticated with the app secret and scoped server-side
// to the app's one bound sheet. Mutating methods read the session user from
// the request context and forward it as the actor for audit attribution.
type SheetsClient struct {
	baseURL string
	secret  string
	httpc   *http.Client
}

func newSheetsClient(baseURL, secret string) *SheetsClient {
	return &SheetsClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		secret:  secret,
		httpc:   &http.Client{Timeout: sheetsRequestTimeout},
	}
}

// Structure fetches the bound sheet's pages, fields, and row counts. Call it
// (or cache it briefly) before building queries — row data and filters key
// by field ID.
func (c *SheetsClient) Structure(ctx context.Context) (*SheetStructure, error) {
	var out SheetStructure
	if err := c.do(ctx, http.MethodGet, "/v1/sheet", false, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// QueryRows runs a filtered, sorted, cursor-paged query against one page.
func (c *SheetsClient) QueryRows(ctx context.Context, pageID string, query Query) (*QueryResult, error) {
	p, err := pagePath(pageID, "/rows/query")
	if err != nil {
		return nil, err
	}
	var out QueryResult
	if err := c.do(ctx, http.MethodPost, p, false, query, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// InsertRows creates up to 100 rows on a page.
func (c *SheetsClient) InsertRows(ctx context.Context, pageID string, rows []RowInsert) ([]Row, error) {
	p, err := pagePath(pageID, "/rows")
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("hivycore: InsertRows needs at least one row")
	}
	var out struct {
		Rows []Row `json:"rows"`
	}
	body := map[string]any{"rows": rows}
	if err := c.do(ctx, http.MethodPost, p, true, body, &out); err != nil {
		return nil, err
	}
	return out.Rows, nil
}

// UpdateRows partial-merges each row's data by field key; a single cell edit
// is a batch of one.
func (c *SheetsClient) UpdateRows(ctx context.Context, pageID string, rows []RowUpdate) ([]Row, error) {
	p, err := pagePath(pageID, "/rows")
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("hivycore: UpdateRows needs at least one row")
	}
	var out struct {
		Rows []Row `json:"rows"`
	}
	body := map[string]any{"rows": rows}
	if err := c.do(ctx, http.MethodPatch, p, true, body, &out); err != nil {
		return nil, err
	}
	return out.Rows, nil
}

// DeleteRows archives rows by ID and returns how many were archived.
func (c *SheetsClient) DeleteRows(ctx context.Context, pageID string, ids []string) (int64, error) {
	p, err := pagePath(pageID, "/rows")
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, errors.New("hivycore: DeleteRows needs at least one id")
	}
	var out struct {
		Archived int64 `json:"archived"`
	}
	body := map[string]any{"ids": ids}
	if err := c.do(ctx, http.MethodDelete, p, true, body, &out); err != nil {
		return 0, err
	}
	return out.Archived, nil
}

// AttachmentDownloadURLs mints short-lived presigned download URLs for
// attachment object keys stored in the page's cells (1–50 keys per call).
func (c *SheetsClient) AttachmentDownloadURLs(ctx context.Context, pageID string, keys []string) (map[string]string, error) {
	p, err := pagePath(pageID, "/attachments/download-url")
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, errors.New("hivycore: AttachmentDownloadURLs needs at least one key")
	}
	var out struct {
		URLs map[string]string `json:"urls"`
	}
	body := map[string]any{"keys": keys}
	if err := c.do(ctx, http.MethodPost, p, false, body, &out); err != nil {
		return nil, err
	}
	return out.URLs, nil
}

func pagePath(pageID, suffix string) (string, error) {
	if strings.TrimSpace(pageID) == "" {
		return "", errors.New("hivycore: pageID is required")
	}
	return "/v1/pages/" + url.PathEscape(pageID) + suffix, nil
}

// do runs one JSON round-trip against the internal app API. withActor pins
// the session user (from ctx) into the actor header on mutations.
func (c *SheetsClient) do(ctx context.Context, method, path string, withActor bool, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("hivycore: encoding sheets request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("hivycore: building sheets request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if withActor {
		if session, ok := UserFrom(ctx); ok && session.UserID != "" {
			req.Header.Set(actorHeader, session.UserID)
		}
	}

	resp, err := c.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("hivycore: sheets api request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	if err != nil {
		return fmt.Errorf("hivycore: reading sheets response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return newAPIError(resp.StatusCode, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("hivycore: decoding sheets response: %w", err)
		}
	}
	return nil
}

func newAPIError(status int, raw []byte) *APIError {
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &body); err == nil && body.Error != "" {
		return &APIError{Status: status, Message: body.Error}
	}
	message := strings.TrimSpace(string(raw))
	if len(message) > 512 {
		message = message[:512]
	}
	if message == "" {
		message = http.StatusText(status)
	}
	return &APIError{Status: status, Message: message}
}
