package agentruntime

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sourcegraph/conc/pool"

	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

const (
	memoryPreloadPerQueryLimit = 100
	memoryPreloadMaxEntries    = 12
)

func buildMemoryContext(ctx context.Context, deps CompileDeps, agent *model.Agent) MemoryContext {
	memory := MemoryContext{Entries: []MemoryContextEntry{}, TokenBudget: 1000}
	if deps.Hindsight == nil || agent == nil || agent.OrgID == nil {
		return memory
	}
	bankID := hindsight.OrgBankID(*agent.OrgID)
	if err := hindsight.RequireBank(ctx, deps.DB, bankID); err != nil {
		logging.Capture(ctx, err)
		return memory
	}
	listCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	queries, err := hindsight.PreloadMemoryListQueries(listCtx, deps.DB, agent)
	if err != nil {
		logging.Capture(ctx, err)
		return memory
	}
	if len(queries) == 0 {
		return memory
	}
	results := listPreloadMemories(listCtx, deps.Hindsight, bankID, queries)
	memory.Entries = compactMemoryResults(results, memoryPreloadMaxEntries, memory.TokenBudget)
	return memory
}

func listPreloadMemories(ctx context.Context, client HindsightMemoryClient, bankID string, queries []hindsight.MemoryListQuery) []any {
	var mu sync.Mutex
	resultsByQuery := make([][]any, len(queries))
	p := pool.New().WithErrors().WithMaxGoroutines(8)
	for index, query := range queries {
		p.Go(func() error {
			resp, err := client.ListMemoriesFiltered(ctx, bankID, hindsight.ListMemoriesOptions{
				Limit:       memoryPreloadPerQueryLimit,
				TagGroups:   query.TagGroups,
				ExcludeTags: query.ExcludeTags,
			})
			if err != nil || resp == nil {
				return err
			}
			items := make([]any, 0, len(resp.Items))
			for _, item := range resp.Items {
				items = append(items, item)
			}
			mu.Lock()
			resultsByQuery[index] = items
			mu.Unlock()
			return nil
		})
	}
	if err := p.Wait(); err != nil {
		logging.Capture(ctx, err)
	}
	results := make([]any, 0, len(queries)*memoryPreloadPerQueryLimit)
	for _, items := range resultsByQuery {
		results = append(results, items...)
	}
	return results
}

func compactMemoryResults(results []any, maxEntries int, tokenBudget int) []MemoryContextEntry {
	entries := make([]MemoryContextEntry, 0, len(results))
	remainingChars := tokenBudget * 4
	seen := map[string]struct{}{}
	for _, raw := range results {
		if len(entries) >= maxEntries || remainingChars <= 0 {
			break
		}
		entry := memoryEntryFromResult(raw)
		entry.Content = strings.TrimSpace(entry.Content)
		if entry.Content == "" {
			continue
		}
		sort.Strings(entry.Tags)
		key := memoryEntryDedupeKey(entry)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if len(entry.Content) > remainingChars {
			entry.Content = entry.Content[:remainingChars]
		}
		remainingChars -= len(entry.Content)
		entries = append(entries, entry)
	}
	return entries
}

func memoryEntryFromResult(raw any) MemoryContextEntry {
	switch value := raw.(type) {
	case string:
		return MemoryContextEntry{Content: value}
	case map[string]any:
		return memoryEntryFromMap(value)
	default:
		bytes, err := json.Marshal(value)
		if err != nil {
			return MemoryContextEntry{}
		}
		var m map[string]any
		if err := json.Unmarshal(bytes, &m); err != nil {
			return MemoryContextEntry{Content: string(bytes)}
		}
		return memoryEntryFromMap(m)
	}
}

func memoryEntryFromMap(m map[string]any) MemoryContextEntry {
	entry := MemoryContextEntry{
		Content:    firstString(m, "content", "text", "memory", "summary", "fact", "observation"),
		Source:     firstString(m, "source", "document_id", "id"),
		MemoryType: firstString(m, "memory_type", "type"),
	}
	if entry.Content == "" {
		if nested, ok := m["document"].(map[string]any); ok {
			entry.Content = firstString(nested, "content", "text", "summary")
		}
	}
	if tags, ok := m["tags"].([]any); ok {
		for _, raw := range tags {
			if tag, ok := raw.(string); ok {
				entry.Tags = append(entry.Tags, tag)
			}
		}
	}
	if tags, ok := m["tags"].([]string); ok {
		entry.Tags = append(entry.Tags, tags...)
	}
	return entry
}

func memoryEntryDedupeKey(entry MemoryContextEntry) string {
	if entry.Source != "" {
		return entry.Source
	}
	return entry.Content
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
