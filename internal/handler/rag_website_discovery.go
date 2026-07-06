package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/rag/connectors/website"
	"github.com/usehivy/hivy/internal/spider"
)

type discoverWebsiteSectionsRequest struct {
	URL string `json:"url"`
}

// DiscoverWebsiteSections handles POST /v1/rag/website/discover-sections.
// @Summary Discover the sections of a website
// @Description Crawls a site's links via Spider and groups them into sections (path clusters) and individual pages, so a website knowledge source can be scoped to a set of URLs.
// @Tags rag
// @Accept json
// @Produce json
// @Param body body discoverWebsiteSectionsRequest true "Website URL"
// @Success 200 {object} website.SectionDiscovery
// @Failure 400 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
// @Router /v1/rag/website/discover-sections [post]
func (h *RAGSourceHandler) DiscoverWebsiteSections(w http.ResponseWriter, r *http.Request) {
	if _, ok := middleware.OrgFromContext(r.Context()); !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	if h.spider == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "website discovery is not configured"})
		return
	}

	var req discoverWebsiteSectionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	target := strings.TrimSpace(req.URL)
	if target == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "url is required"})
		return
	}
	if !strings.Contains(target, "://") {
		target = "https://" + target
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	// discoveryPageLimit caps how many pages Spider crawls to enumerate links —
	// enough to surface the site's sections without a full crawl.
	const discoveryPageLimit = 300
	limit := discoveryPageLimit
	pages, err := h.spider.Links(ctx, spider.SpiderParams{
		URL:             target,
		RequestType:     "smart",
		Limit:           &limit,
		ReturnPageLinks: boolPtr(true),
		RespectRobots:   boolPtr(true),
	})
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "website discovery: spider links failed", "url", target, "error", err)
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to crawl site: " + err.Error()})
		return
	}

	// Aggregate every discovered URL: crawled page URLs plus the links on them.
	set := make(map[string]struct{}, len(pages)*8)
	for _, p := range pages {
		if p.URL != "" {
			set[p.URL] = struct{}{}
		}
		for _, link := range p.Links {
			if link != "" {
				set[link] = struct{}{}
			}
		}
	}
	all := make([]string, 0, len(set))
	for u := range set {
		all = append(all, u)
	}

	result, err := website.GroupLinks(target, all, 0)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}
