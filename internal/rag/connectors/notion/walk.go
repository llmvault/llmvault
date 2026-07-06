package notion

import (
	"context"
	"fmt"
	"strings"
)

// blockClient is the subset of the API the block walker needs. Kept as
// an interface so tests can drive the traversal with fixture data.
type blockClient interface {
	blockChildren(ctx context.Context, blockID, cursor string) (searchResult, bool, error)
	dataSources(ctx context.Context, databaseID string) ([]NotionDataSource, error)
	dataSourceQuery(ctx context.Context, dataSourceID, cursor string) (dataSourceQueryResult, error)
}

// walker walks a page's block tree into rendered text. It carries the
// cross-page bookkeeping maps that must persist for the length of a run:
// which child page belongs to which containing page, and which data
// source belongs to which database.
type walker struct {
	client           blockClient
	includeDatabases bool

	childPageParentMap      map[string]string
	dataSourceToDatabaseMap map[string]string
}

func newWalker(client blockClient, includeDatabases bool) *walker {
	return &walker{
		client:                  client,
		includeDatabases:        includeDatabases,
		childPageParentMap:      map[string]string{},
		dataSourceToDatabaseMap: map[string]string{},
	}
}

// workItem is one entry on the explicit DFS stack. A process item
// expands a block's own children; a finalize item emits the block's
// database content (if any) and its own text after its subtree, giving
// post-order text. closeID, when set, is the ancestor id to release from
// the open set once the block's subtree is done.
type workItem struct {
	result   map[string]any
	finalize bool
	text     []string
	closeID  string
}

// readBlocks walks the block tree rooted at baseBlockID iteratively.
// Iteration (not recursion) keeps deeply nested pages from overflowing
// the goroutine stack; openBlockIDs breaks reference cycles (e.g. synced
// blocks that nest an ancestor). containingPageID is the page these
// blocks belong to and is used to map discovered child pages to their
// real parent rather than to an intermediate block.
func (w *walker) readBlocks(ctx context.Context, baseBlockID, containingPageID string) (blockReadOutput, error) {
	pageID := containingPageID
	if pageID == "" {
		pageID = baseBlockID
	}

	var resultBlocks []NotionBlock
	var childPages []string
	openBlockIDs := map[string]struct{}{baseBlockID: {}}

	firstChildren, err := w.fetchAllChildBlocks(ctx, baseBlockID)
	if err != nil {
		return blockReadOutput{}, err
	}

	stack := make([]workItem, 0, len(firstChildren))
	for i := len(firstChildren) - 1; i >= 0; i-- {
		stack = append(stack, workItem{result: firstChildren[i]})
	}

	for len(stack) > 0 {
		if ctx.Err() != nil {
			return blockReadOutput{}, ctx.Err()
		}

		item := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if item.finalize {
			if item.closeID != "" {
				delete(openBlockIDs, item.closeID)
			}
			blockID := getString(item.result, "id")
			blockType := getString(item.result, "type")

			// A database rendered on a page reads like a table: include
			// its row text here, and queue the row pages as their own
			// documents. Gated entirely on the config toggle.
			if blockType == "child_database" && w.includeDatabases {
				dbOut, err := w.readPagesFromDatabase(ctx, blockID)
				if err != nil {
					return blockReadOutput{}, err
				}
				resultBlocks = append(resultBlocks, dbOut.Blocks...)
				childPages = append(childPages, dbOut.ChildPageIDs...)
			}

			if len(item.text) > 0 {
				resultBlocks = append(resultBlocks, NotionBlock{
					ID:     blockID,
					Text:   strings.Join(item.text, "\n"),
					Prefix: "\n",
				})
			}
			continue
		}

		result := item.result
		blockID := getString(result, "id")
		blockType := getString(result, "type")

		// Block types the API cannot render are skipped outright.
		switch blockType {
		case "ai_block", "unsupported", "external_object_instance_page":
			continue
		}

		textParts := blockText(result)

		willRecurse := false
		if getBool(result, "has_children") {
			switch {
			case blockType == "child_page":
				// Child pages are separate documents, not inlined here.
				// Record their containing page for parent resolution.
				childPages = append(childPages, blockID)
				w.childPageParentMap[blockID] = pageID
			case isOpen(openBlockIDs, blockID):
				// Cycle: this block is already an ancestor being expanded.
			default:
				willRecurse = true
			}
		}

		if willRecurse {
			openBlockIDs[blockID] = struct{}{}
			// Push the finalize item first so it pops after the whole
			// subtree — descendants emit before this block's own text.
			stack = append(stack, workItem{result: result, finalize: true, text: textParts, closeID: blockID})
			children, err := w.fetchAllChildBlocks(ctx, blockID)
			if err != nil {
				return blockReadOutput{}, err
			}
			for i := len(children) - 1; i >= 0; i-- {
				stack = append(stack, workItem{result: children[i]})
			}
		} else {
			stack = append(stack, workItem{result: result, finalize: true, text: textParts})
		}
	}

	return blockReadOutput{Blocks: resultBlocks, ChildPageIDs: childPages}, nil
}

// fetchAllChildBlocks pages through every child of blockID in order. A
// not-shared response (bool false) ends collection with whatever was
// gathered so far.
func (w *walker) fetchAllChildBlocks(ctx context.Context, blockID string) ([]map[string]any, error) {
	var out []map[string]any
	cursor := ""
	for {
		res, ok, err := w.client.blockChildren(ctx, blockID, cursor)
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		out = append(out, res.Results...)
		if res.NextCursor == nil || *res.NextCursor == "" {
			break
		}
		cursor = *res.NextCursor
	}
	return out, nil
}

// readPagesFromDatabase queries every data source under a database and
// renders each row's properties to text. Row pages (and nested database
// rows) are collected as child page ids for later indexing.
func (w *walker) readPagesFromDatabase(ctx context.Context, databaseID string) (blockReadOutput, error) {
	var blocks []NotionBlock
	var pages []string

	dataSources, err := w.client.dataSources(ctx, databaseID)
	if err != nil {
		return blockReadOutput{}, err
	}

	for _, ds := range dataSources {
		w.dataSourceToDatabaseMap[ds.ID] = databaseID
		cursor := ""
		for {
			res, err := w.client.dataSourceQuery(ctx, ds.ID, cursor)
			if err != nil {
				return blockReadOutput{}, err
			}
			for _, row := range res.Results {
				objID := getString(row, "id")
				objType := getString(row, "object")
				props, _ := row["properties"].(map[string]any)
				if text := propertiesToStr(props); text != "" {
					blocks = append(blocks, NotionBlock{ID: objID, Text: text, Prefix: "\n"})
				}

				switch objType {
				case "page":
					pages = append(pages, objID)
				case "database":
					nested, err := w.readPagesFromDatabase(ctx, objID)
					if err != nil {
						return blockReadOutput{}, err
					}
					pages = append(pages, nested.ChildPageIDs...)
				}
			}
			if res.NextCursor == nil || *res.NextCursor == "" {
				break
			}
			cursor = *res.NextCursor
		}
	}

	return blockReadOutput{Blocks: blocks, ChildPageIDs: pages}, nil
}

// ---- block/property text extraction ----

// blockText pulls the renderable text out of a block. Regular blocks
// carry a rich_text array; table_row blocks carry a cells matrix
// (list-of-lists of rich text) which is tab-joined per row.
func blockText(m map[string]any) []string {
	obj := blockTypeObj(m)
	if obj == nil {
		return nil
	}

	var parts []string
	if rich, ok := obj["rich_text"].([]any); ok {
		for _, r := range rich {
			rm, ok := r.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := rm["text"].(map[string]any); ok {
				if content, ok := text["content"].(string); ok {
					parts = append(parts, content)
				}
			}
		}
	}

	if cells, ok := obj["cells"].([]any); ok {
		rowParts := make([]string, 0, len(cells))
		for _, cell := range cells {
			cellArr, _ := cell.([]any)
			words := make([]string, 0, len(cellArr))
			for _, rt := range cellArr {
				rtm, ok := rt.(map[string]any)
				if !ok {
					continue
				}
				if pt, ok := rtm["plain_text"].(string); ok {
					words = append(words, pt)
				}
			}
			rowParts = append(rowParts, strings.Join(words, " "))
		}
		parts = append(parts, strings.Join(rowParts, "\t"))
	}

	return parts
}

// propertiesToStr renders a page/row property map to a flat "Name: value"
// string. Keys are emitted in sorted order for deterministic output.
func propertiesToStr(properties map[string]any) string {
	if len(properties) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, name := range sortedKeys(properties) {
		prop, ok := properties[name].(map[string]any)
		if !ok {
			continue
		}
		if value, ok := recurseProperty(prop); ok && value != "" {
			sb.WriteString(name)
			sb.WriteString(": ")
			sb.WriteString(value)
			sb.WriteString("\t")
		}
	}
	return sb.String()
}

// recurseProperty resolves a single property's scalar value. It descends
// through the type-tagged wrappers Notion uses, capturing a user's
// "name" (people/created_by/etc.) before drilling past it, rendering
// date ranges as "start - end", and pulling name/content out of the
// innermost object. Returns ok=false when nothing indexable is present.
func recurseProperty(v any) (string, bool) {
	cur := v
	for {
		m, ok := cur.(map[string]any)
		if !ok {
			break
		}
		typeName, ok := m["type"].(string)
		if !ok {
			break
		}
		// User objects carry "name" alongside "type": "person"/"bot".
		// Capture it before descending, but not for title properties
		// where "name" is not the display value.
		if name, ok := m["name"].(string); ok && typeName != "title" {
			return name, true
		}
		next, exists := m[typeName]
		if !exists {
			break
		}
		cur = next
		if isEmptyValue(cur) {
			return "", false
		}
	}

	switch t := cur.(type) {
	case []any:
		return recursePropertyList(t)
	case string:
		return t, t != ""
	case map[string]any:
		if name, ok := t["name"].(string); ok {
			return name, name != ""
		}
		if content, ok := t["content"].(string); ok {
			return content, content != ""
		}
		start, hasStart := t["start"].(string)
		end, hasEnd := t["end"].(string)
		if hasStart && start != "" {
			if hasEnd && end != "" {
				return start + " - " + end, true
			}
			return start, true
		}
		if hasEnd && end != "" {
			return "Until " + end, true
		}
		// A bare id reference is not useful in plaintext.
		if _, ok := t["id"]; ok {
			return "", false
		}
	}
	return "", false
}

func recursePropertyList(items []any) (string, bool) {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		switch it := item.(type) {
		case map[string]any:
			if s, ok := recurseProperty(it); ok && s != "" {
				parts = append(parts, s)
			}
		case []any:
			if s, ok := recursePropertyList(it); ok && s != "" {
				parts = append(parts, s)
			}
		default:
			if it != nil {
				parts = append(parts, fmt.Sprint(it))
			}
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, ", "), true
}

// readPageTitle extracts a page's title: the database name for a wiki,
// otherwise the plain_text of the first title-typed property. Returns ""
// when no title is present so callers can apply a fallback.
func readPageTitle(page NotionPage) string {
	if page.DatabaseName != "" {
		return page.DatabaseName
	}
	for _, key := range sortedKeys(page.Properties) {
		prop, ok := page.Properties[key].(map[string]any)
		if !ok {
			continue
		}
		if getString(prop, "type") != "title" {
			continue
		}
		titleArr, ok := prop["title"].([]any)
		if !ok || len(titleArr) == 0 {
			continue
		}
		parts := make([]string, 0, len(titleArr))
		for _, t := range titleArr {
			tm, ok := t.(map[string]any)
			if !ok {
				continue
			}
			if pt, ok := tm["plain_text"].(string); ok {
				parts = append(parts, pt)
			}
		}
		return strings.TrimSpace(strings.Join(parts, " "))
	}
	return ""
}
