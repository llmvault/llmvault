package notion

import (
	"strings"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// docIDForPage is the stable, prune-friendly document identifier for a
// page. The id is normalised to canonical UUID form so the same page
// keys identically whether discovered via search or as a child.
func docIDForPage(pageID string) string {
	return "notion_page_" + normalizeNotionID(pageID)
}

// pageToDocument renders a page and its walked blocks into the neutral
// Document shape. Each block becomes one section linking to that block's
// anchor; an empty page falls back to a single section carrying the
// title and a property dump. Notion has no per-page ACL, so every
// document is org-wide readable (IsPublic, empty ACL).
func pageToDocument(page NotionPage, blocks []NotionBlock) interfaces.Document {
	rawTitle := readPageTitle(page)
	title := rawTitle
	if title == "" {
		title = "Untitled Page " + page.ID
	}

	var sections []interfaces.Section
	if len(blocks) == 0 {
		sections = []interfaces.Section{{Text: emptyPageText(title, page.Properties), Link: page.URL}}
	} else {
		sections = make([]interfaces.Section, 0, len(blocks))
		for _, b := range blocks {
			sections = append(sections, interfaces.Section{
				Text: b.Prefix + b.Text,
				Link: page.URL + "#" + stripDashes(b.ID),
			})
		}
	}

	return interfaces.Document{
		DocID:        docIDForPage(page.ID),
		SemanticID:   title,
		Link:         page.URL,
		Sections:     sections,
		IsPublic:     true,
		DocUpdatedAt: parseNotionTime(page.LastEditedTime),
		Metadata:     map[string]string{"object_type": "page"},
	}
}

// emptyPageText builds the fallback body for a page with no renderable
// blocks: the title followed by a "key: value" dump of its properties.
func emptyPageText(title string, properties map[string]any) string {
	if len(properties) == 0 {
		return title
	}
	var sb strings.Builder
	sb.WriteString(title)
	sb.WriteString("\n")
	for _, key := range sortedKeys(properties) {
		value := ""
		if prop, ok := properties[key].(map[string]any); ok {
			value, _ = recurseProperty(prop)
		}
		sb.WriteString("\n")
		sb.WriteString(key)
		sb.WriteString(": ")
		sb.WriteString(value)
	}
	return sb.String()
}
