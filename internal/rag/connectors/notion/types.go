package notion

// notionVersion is the Notion API version this connector is pinned to.
// Sent on every request via the Notion-Version header.
const notionVersion = "2026-03-11"

// NotionPage is a page (or database-rendered-as-page) returned by the
// pages/databases/search endpoints. Properties are kept as a loose map
// because their shape is polymorphic per property type.
type NotionPage struct {
	ID             string         `json:"id"`
	Object         string         `json:"object,omitempty"`
	CreatedTime    string         `json:"created_time,omitempty"`
	LastEditedTime string         `json:"last_edited_time,omitempty"`
	InTrash        bool           `json:"in_trash,omitempty"`
	Properties     map[string]any `json:"properties,omitempty"`
	URL            string         `json:"url,omitempty"`
	Parent         map[string]any `json:"parent,omitempty"`

	// DatabaseName is populated only when a page id actually resolved to
	// a database (a "wiki") — the title moves out of Properties in that
	// case and is captured here instead.
	DatabaseName string `json:"-"`
}

// NotionDataSource identifies one data source within a database. Even
// legacy single-source databases expose exactly one entry.
type NotionDataSource struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// NotionBlock is one rendered unit of page text. Prefix records how the
// block joins the running plaintext (always a newline here); it is kept
// separate from Text so the document builder can compose sections.
type NotionBlock struct {
	ID     string
	Text   string
	Prefix string
}

// blockReadOutput is the result of walking a page's (or database's)
// block tree: the rendered blocks in emit order plus the ids of child
// pages discovered along the way (collected, never inlined).
type blockReadOutput struct {
	Blocks       []NotionBlock
	ChildPageIDs []string
}

// searchResult is the shape of a /v1/search response.
type searchResult struct {
	Results    []map[string]any `json:"results"`
	NextCursor *string          `json:"next_cursor"`
	HasMore    bool             `json:"has_more"`
}

// dataSourceQueryResult is the shape of a /v1/data_sources/{id}/query
// response. Rows are kept loose because they may be page or database
// objects with polymorphic properties.
type dataSourceQueryResult struct {
	Results    []map[string]any `json:"results"`
	NextCursor *string          `json:"next_cursor"`
}
