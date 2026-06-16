package hindsight

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

func (c *Client) ListMemories(ctx context.Context, bankID string, limit, offset int) (*ListMemoriesResponse, error) {
	return c.ListMemoriesFiltered(ctx, bankID, ListMemoriesOptions{Limit: limit, Offset: offset})
}

func (c *Client) ListMemoriesFiltered(ctx context.Context, bankID string, opts ListMemoriesOptions) (*ListMemoriesResponse, error) {
	path, err := listMemoriesPath(bankID, opts)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hindsight list memories: status %d: %s", resp.StatusCode, string(body))
	}

	var result ListMemoriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("hindsight list memories: decoding response: %w", err)
	}
	return &result, nil
}

func listMemoriesPath(bankID string, opts ListMemoriesOptions) (string, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}
	values := url.Values{}
	values.Set("limit", strconv.Itoa(limit))
	values.Set("offset", strconv.Itoa(offset))
	if len(opts.TagGroups) > 0 {
		encoded, err := json.Marshal(opts.TagGroups)
		if err != nil {
			return "", fmt.Errorf("encode memory tag groups: %w", err)
		}
		values.Set("tag_groups", string(encoded))
	}
	if len(opts.ExcludeTags) > 0 {
		encoded, err := json.Marshal(opts.ExcludeTags)
		if err != nil {
			return "", fmt.Errorf("encode memory exclude tags: %w", err)
		}
		values.Set("exclude_tags", string(encoded))
	}
	return fmt.Sprintf("/v1/default/banks/%s/memories/list?%s", url.PathEscape(bankID), values.Encode()), nil
}
