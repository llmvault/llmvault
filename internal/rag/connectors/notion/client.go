package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// Client is the typed Notion API surface used by the connector. Each
// method wraps a single logical request in withRetry. Responses of 403
// or 404 are treated as "not shared with the integration" and surfaced
// as empty results rather than errors, so an unshared page never aborts
// a run.
type Client struct {
	p proxyClient
}

func newClient(p proxyClient) *Client {
	return &Client{p: p}
}

// do performs one request with retries. It returns the response body and
// status for the caller to interpret. Transport failures and 5xx
// responses are retried; 4xx responses are returned to the caller (which
// decides tolerability); 429 responses trigger a rate-limit wait.
func (c *Client) do(ctx context.Context, method, path, rawQuery string, body []byte) ([]byte, int, error) {
	var respBody []byte
	var status int

	err := withRetry(ctx, func() error {
		st, hdr, b, e := c.p.Do(ctx, method, path, rawQuery, body)
		if e != nil {
			return e
		}
		if resetAt, ok := detectRateLimit(st, hdr); ok {
			return &rateLimitError{retryAt: resetAt}
		}
		if st >= 500 {
			return fmt.Errorf("notion %s %s: status %d: %s", method, path, st, preview(b))
		}
		status = st
		respBody = b
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return respBody, status, nil
}

// tolerated reports whether a status is an "unshared resource" response
// the connector swallows into empty output.
func tolerated(status int) bool {
	return status == http.StatusForbidden || status == http.StatusNotFound
}

func preview(b []byte) string {
	if len(b) > 256 {
		b = b[:256]
	}
	return string(b)
}

// usersMe returns the bot user record. Used as a cheap workspace-mode
// reachability probe during config validation.
func (c *Client) usersMe(ctx context.Context) (map[string]any, error) {
	body, status, err := c.do(ctx, http.MethodGet, "/v1/users/me", "", nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("notion users/me: status %d: %s", status, preview(body))
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("notion users/me: decode: %w", err)
	}
	return out, nil
}

// search runs a POST /v1/search. filterObject scopes results to a single
// object type (e.g. "page"); when sortDesc is set, results come back
// newest-edited first so callers can early-break an incremental scan.
func (c *Client) search(ctx context.Context, filterObject string, sortDesc bool, cursor *string) (searchResult, error) {
	reqBody := map[string]any{
		"filter":    map[string]any{"property": "object", "value": filterObject},
		"page_size": notionPageSize,
	}
	if sortDesc {
		reqBody["sort"] = map[string]any{"timestamp": "last_edited_time", "direction": "descending"}
	}
	if cursor != nil && *cursor != "" {
		reqBody["start_cursor"] = *cursor
	}

	raw, err := json.Marshal(reqBody)
	if err != nil {
		return searchResult{}, fmt.Errorf("notion search: encode: %w", err)
	}

	respBody, status, err := c.do(ctx, http.MethodPost, "/v1/search", "", raw)
	if err != nil {
		return searchResult{}, err
	}
	if status >= 400 {
		return searchResult{}, fmt.Errorf("notion search: status %d: %s", status, preview(respBody))
	}
	var res searchResult
	if err := json.Unmarshal(respBody, &res); err != nil {
		return searchResult{}, fmt.Errorf("notion search: decode: %w", err)
	}
	return res, nil
}

// blockChildren returns one page of a block's children. The bool result
// is false when the block is not shared with the integration (403/404),
// in which case the caller should stop collecting children.
func (c *Client) blockChildren(ctx context.Context, blockID, cursor string) (searchResult, bool, error) {
	q := url.Values{}
	if cursor != "" {
		q.Set("start_cursor", cursor)
	}
	body, status, err := c.do(ctx, http.MethodGet, "/v1/blocks/"+blockID+"/children", q.Encode(), nil)
	if err != nil {
		return searchResult{}, false, err
	}
	if tolerated(status) {
		return searchResult{}, false, nil
	}
	if status >= 400 {
		return searchResult{}, false, fmt.Errorf("notion blocks/%s/children: status %d: %s", blockID, status, preview(body))
	}
	var res searchResult
	if err := json.Unmarshal(body, &res); err != nil {
		return searchResult{}, false, fmt.Errorf("notion blocks/%s/children: decode: %w", blockID, err)
	}
	return res, true, nil
}

// page fetches a page by id. Because a page id may actually reference a
// database ("wiki"), a failed page read falls back to a database read.
// The bool result is false when neither is shared with the integration.
func (c *Client) page(ctx context.Context, pageID string) (NotionPage, bool, error) {
	body, status, err := c.do(ctx, http.MethodGet, "/v1/pages/"+pageID, "", nil)
	if err == nil && status < 400 {
		var p NotionPage
		if e := json.Unmarshal(body, &p); e != nil {
			return NotionPage{}, false, fmt.Errorf("notion pages/%s: decode: %w", pageID, e)
		}
		return p, true, nil
	}
	return c.databaseAsPage(ctx, pageID)
}

// databaseAsPage reads a database and adapts it into a NotionPage. As of
// recent API versions a database no longer carries `properties` (the
// schema moved to its data sources), so the title is lifted out of the
// top-level `title` array into DatabaseName.
func (c *Client) databaseAsPage(ctx context.Context, databaseID string) (NotionPage, bool, error) {
	body, status, err := c.do(ctx, http.MethodGet, "/v1/databases/"+databaseID, "", nil)
	if err != nil {
		return NotionPage{}, false, err
	}
	if tolerated(status) {
		return NotionPage{}, false, nil
	}
	if status >= 400 {
		return NotionPage{}, false, fmt.Errorf("notion databases/%s: status %d: %s", databaseID, status, preview(body))
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return NotionPage{}, false, fmt.Errorf("notion databases/%s: decode: %w", databaseID, err)
	}
	var p NotionPage
	if err := json.Unmarshal(body, &p); err != nil {
		return NotionPage{}, false, fmt.Errorf("notion databases/%s: decode: %w", databaseID, err)
	}
	if p.Properties == nil {
		p.Properties = map[string]any{}
	}
	p.DatabaseName = databaseTitle(raw)
	return p, true, nil
}

// dataSources lists the data sources under a database. A 403/404 yields
// an empty list (database not shared) rather than an error.
func (c *Client) dataSources(ctx context.Context, databaseID string) ([]NotionDataSource, error) {
	body, status, err := c.do(ctx, http.MethodGet, "/v1/databases/"+databaseID, "", nil)
	if err != nil {
		return nil, err
	}
	if tolerated(status) {
		return nil, nil
	}
	if status >= 400 {
		return nil, fmt.Errorf("notion databases/%s: status %d: %s", databaseID, status, preview(body))
	}

	var resp struct {
		DataSources []NotionDataSource `json:"data_sources"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("notion databases/%s: decode: %w", databaseID, err)
	}
	out := make([]NotionDataSource, 0, len(resp.DataSources))
	for _, ds := range resp.DataSources {
		if ds.ID != "" {
			out = append(out, ds)
		}
	}
	return out, nil
}

// dataSourceQuery returns one page of rows from a data source. A 403/404
// yields an empty result (source not shared) rather than an error.
func (c *Client) dataSourceQuery(ctx context.Context, dataSourceID, cursor string) (dataSourceQueryResult, error) {
	var body []byte
	if cursor != "" {
		body, _ = json.Marshal(map[string]any{"start_cursor": cursor})
	}
	respBody, status, err := c.do(ctx, http.MethodPost, "/v1/data_sources/"+dataSourceID+"/query", "", body)
	if err != nil {
		return dataSourceQueryResult{}, err
	}
	if tolerated(status) {
		return dataSourceQueryResult{}, nil
	}
	if status >= 400 {
		return dataSourceQueryResult{}, fmt.Errorf("notion data_sources/%s/query: status %d: %s", dataSourceID, status, preview(respBody))
	}
	var res dataSourceQueryResult
	if err := json.Unmarshal(respBody, &res); err != nil {
		return dataSourceQueryResult{}, fmt.Errorf("notion data_sources/%s/query: decode: %w", dataSourceID, err)
	}
	return res, nil
}

// databaseTitle extracts a database's display name from its top-level
// `title` rich-text array, preferring plain_text then text.content.
func databaseTitle(raw map[string]any) string {
	arr, ok := raw["title"].([]any)
	if !ok || len(arr) == 0 {
		return ""
	}
	first, ok := arr[0].(map[string]any)
	if !ok {
		return ""
	}
	if pt, ok := first["plain_text"].(string); ok && pt != "" {
		return pt
	}
	if text, ok := first["text"].(map[string]any); ok {
		if content, ok := text["content"].(string); ok {
			return content
		}
	}
	return ""
}
