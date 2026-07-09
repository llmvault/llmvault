// Package website is a RAG connector that crawls a public website via the
// configured webcrawl.Provider and emits one Document per page in markdown.
//
// It scrapes each configured URL to markdown; MaxPages caps the list
// (default 500). Subdomains and link-budget tunables stay zero-valued for v1;
// revisit when callers need them.
package website

import (
	"context"
	"fmt"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
	"github.com/usehivy/hivy/internal/webcrawl"
)

const Kind = "WEBSITE"

var _ interfaces.Connector = (*WebsiteConnector)(nil)

type WebsiteConnector struct {
	cfg WebsiteConfig
	web webcrawl.Provider
}

func NewConnector(cfg WebsiteConfig, web webcrawl.Provider) *WebsiteConnector {
	return &WebsiteConnector{cfg: cfg, web: web}
}

func (c *WebsiteConnector) Kind() string { return Kind }

func (c *WebsiteConnector) ValidateConfig(_ context.Context, src interfaces.Source) error {
	_, err := LoadConfig(src.Config())
	return err
}

func Build(src interfaces.Source, deps interfaces.BuildDeps) (interfaces.Connector, error) {
	cfg, err := LoadConfig(src.Config())
	if err != nil {
		return nil, err
	}
	if deps.Web == nil {
		return nil, fmt.Errorf("website: web crawl provider not configured")
	}
	return NewConnector(cfg, deps.Web), nil
}
