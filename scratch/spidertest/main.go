// Scratch tool: validate the website section-discovery algorithm against a real
// site via Spider. Run: HIVY_SPIDER_CLOUD_API_KEY=... go run ./scratch/spidertest <url>
// Delete after validation.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/usehivy/hivy/internal/rag/connectors/website"
	"github.com/usehivy/hivy/internal/spider"
)

func main() {
	target := "https://vercel.com"
	if len(os.Args) > 1 {
		target = os.Args[1]
	}
	key := os.Getenv("HIVY_SPIDER_CLOUD_API_KEY")
	if key == "" {
		fmt.Println("HIVY_SPIDER_CLOUD_API_KEY not set")
		os.Exit(1)
	}
	base := os.Getenv("HIVY_SPIDER_BASE_URL")
	if base == "" {
		base = "https://api.spider.cloud"
	}

	client := spider.NewClient(base, key)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	limit := 300
	yes := true
	fmt.Printf("crawling %s (limit %d) via %s ...\n", target, limit, base)
	pages, err := client.Links(ctx, spider.SpiderParams{
		URL:             target,
		RequestType:     "smart",
		Limit:           &limit,
		ReturnPageLinks: &yes,
		RespectRobots:   &yes,
	})
	if err != nil {
		fmt.Printf("spider.Links error: %v\n", err)
		os.Exit(1)
	}

	set := map[string]struct{}{}
	for _, p := range pages {
		if p.URL != "" {
			set[p.URL] = struct{}{}
		}
		for _, l := range p.Links {
			if l != "" {
				set[l] = struct{}{}
			}
		}
	}
	all := make([]string, 0, len(set))
	for u := range set {
		all = append(all, u)
	}
	fmt.Printf("spider returned %d pages, %d unique links\n\n", len(pages), len(all))

	result, err := website.GroupLinks(target, all, 0)
	if err != nil {
		fmt.Printf("GroupLinks error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("BASE: %s\n\nSECTIONS (%d):\n", result.BaseURL, len(result.Sections))
	for _, s := range result.Sections {
		fmt.Printf("  • %-22s %-28s %4d pages   e.g. %v\n", s.Label, s.PathPrefix, s.PageCount, s.SamplePaths)
	}
	fmt.Printf("\nINDIVIDUAL PAGES (%d):\n", len(result.Pages))
	for i, p := range result.Pages {
		if i >= 40 {
			fmt.Printf("  ... and %d more\n", len(result.Pages)-40)
			break
		}
		fmt.Printf("  • %s\n", p.Path)
	}
}
