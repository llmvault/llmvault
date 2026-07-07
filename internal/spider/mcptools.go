package spider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

// NewWebToolsFunc returns a callback compatible with mcpserver.WebToolsFunc.
func NewWebToolsFunc(client *Client) func(server *mcp.Server, token *model.Token) {
	return func(server *mcp.Server, token *model.Token) {
		registerWebFetch(server, client)
		registerWebSearch(server, client)
		registerWebCrawl(server, client)
	}
}

func registerWebFetch(server *mcp.Server, client *Client) {
	server.AddTool(
		&mcp.Tool{
			Name: "web_fetch",
			Description: `Fetch a URL and return its content. Converts the page to markdown by default, making it easy to read and process. Use this tool when you need to:
- Read the contents of a specific webpage
- Extract text content from a URL for analysis
- Get documentation or article content
- Verify what a page contains before taking further action

Returns the page content as text (markdown by default).`,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "The URL to fetch. Must be a valid HTTP or HTTPS URL.",
					},
					"return_format": map[string]any{
						"type":        "string",
						"enum":        []string{"markdown", "text", "html"},
						"description": "Output format. 'markdown' (default) converts HTML to clean markdown. 'text' strips all markup. 'html' returns raw HTML.",
					},
				},
				"required": []string{"url"},
			},
		},
		WebFetchHandler(client),
	)
}

func registerWebSearch(server *mcp.Server, client *Client) {
	server.AddTool(
		&mcp.Tool{
			Name: "web_search",
			Description: `Search the web and return a list of results with titles, descriptions, and URLs. Use this tool when you need to:
- Find current information not in your training data
- Look up recent events, documentation, or references
- Research a topic before answering a question
- Find URLs to then fetch with web_fetch for deeper reading

For content from the company's own synced websites or docs, check search_knowledge_base first: crawled pages of synced sites are indexed there. Use web search for the open web, sites that are not synced, or freshly published pages (sync lag).

Returns an array of search results, each with url, title, and description.`,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The search query. Be specific for better results.",
					},
					"num": map[string]any{
						"type":        "integer",
						"description": "Number of results to return (default 5, max 20).",
					},
				},
				"required": []string{"query"},
			},
		},
		WebSearchHandler(client),
	)
}

func registerWebCrawl(server *mcp.Server, client *Client) {
	server.AddTool(
		&mcp.Tool{
			Name: "web_crawl",
			Description: `Crawl a website starting from a URL, following links to gather content from multiple pages. Converts each page to markdown by default. Use this tool when you need to:
- Gather content from many pages of a site or documentation set at once
- Explore a site broadly rather than reading a single known page
- Collect a corpus of related pages for deeper research

Prefer web_search to discover URLs and web_fetch to read one known page. Reach for web_crawl only when you need breadth across a site. If the site is already synced into the knowledge base, prefer search_knowledge_base over re-crawling; crawl sites that are not synced or when you need live page content. Returns an array of pages, each with url, content, and status.`,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "The starting URL to crawl from. Must be a valid HTTP or HTTPS URL.",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of pages to crawl (default 10, max 100). Keep this small to control cost and latency.",
					},
					"depth": map[string]any{
						"type":        "integer",
						"description": "Maximum link depth to follow from the starting URL (default 2).",
					},
					"return_format": map[string]any{
						"type":        "string",
						"enum":        []string{"markdown", "text", "html"},
						"description": "Output format for each page. 'markdown' (default) converts HTML to clean markdown. 'text' strips all markup. 'html' returns raw HTML.",
					},
				},
				"required": []string{"url"},
			},
		},
		WebCrawlHandler(client),
	)
}

// WebFetchHandler returns an MCP tool handler that fetches a URL via Spider.
func WebFetchHandler(client *Client) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var params struct {
			URL          string `json:"url"`
			ReturnFormat string `json:"return_format"`
		}
		if req.Params.Arguments != nil {
			_ = json.Unmarshal(req.Params.Arguments, &params)
		}
		if params.URL == "" {
			return toolError("url is required"), nil
		}
		if params.ReturnFormat == "" {
			params.ReturnFormat = "markdown"
		}

		results, err := client.Crawl(ctx, SpiderParams{
			URL:          params.URL,
			ReturnFormat: params.ReturnFormat,
			RequestType:  "smart",
		})
		if err != nil {
			return toolError("web fetch failed: " + err.Error()), nil
		}
		if len(results) == 0 {
			return toolError("no content returned for URL"), nil
		}
		if results[0].Error != "" {
			return toolError("web fetch error: " + results[0].Error), nil
		}

		return toolJSON(map[string]any{
			"url":     results[0].URL,
			"content": results[0].Content,
			"status":  results[0].StatusCode,
		})
	}
}

// WebSearchHandler returns an MCP tool handler that performs a web search via Spider.
func WebSearchHandler(client *Client) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var params struct {
			Query string `json:"query"`
			Num   *int   `json:"num"`
		}
		if req.Params.Arguments != nil {
			_ = json.Unmarshal(req.Params.Arguments, &params)
		}
		if params.Query == "" {
			return toolError("query is required"), nil
		}

		sp := SearchParams{
			SpiderParams: SpiderParams{
				RequestType: "smart",
			},
			Search: params.Query,
		}
		if params.Num != nil {
			sp.Num = params.Num
		}

		results, err := client.Search(ctx, sp)
		if err != nil {
			return toolError("web search failed: " + err.Error()), nil
		}

		return toolJSON(results.Content)
	}
}

// WebCrawlHandler returns an MCP tool handler that crawls a website via Spider.
func WebCrawlHandler(client *Client) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var params struct {
			URL          string `json:"url"`
			Limit        *int   `json:"limit"`
			Depth        *int   `json:"depth"`
			ReturnFormat string `json:"return_format"`
		}
		if req.Params.Arguments != nil {
			_ = json.Unmarshal(req.Params.Arguments, &params)
		}
		if params.URL == "" {
			return toolError("url is required"), nil
		}
		if params.ReturnFormat == "" {
			params.ReturnFormat = "markdown"
		}

		limit := 10
		if params.Limit != nil && *params.Limit > 0 {
			limit = *params.Limit
		}
		if limit > 100 {
			limit = 100
		}

		sp := SpiderParams{
			URL:          params.URL,
			ReturnFormat: params.ReturnFormat,
			RequestType:  "smart",
			Limit:        &limit,
		}
		if params.Depth != nil {
			sp.Depth = params.Depth
		}

		results, err := client.Crawl(ctx, sp)
		if err != nil {
			return toolError("web crawl failed: " + err.Error()), nil
		}
		if len(results) == 0 {
			return toolError("no content returned for crawl"), nil
		}

		pages := make([]map[string]any, 0, len(results))
		for _, result := range results {
			if result.Error != "" {
				continue
			}
			pages = append(pages, map[string]any{
				"url":     result.URL,
				"content": result.Content,
				"status":  result.StatusCode,
			})
		}
		if len(pages) == 0 {
			return toolError("all crawled pages returned errors"), nil
		}

		return toolJSON(pages)
	}
}

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %s", msg)}},
		IsError: true,
	}
}

func toolJSON(v any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return toolError("failed to serialize response"), nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}, nil
}
