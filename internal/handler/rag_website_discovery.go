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
	"github.com/usehivy/hivy/internal/webcrawl"
)

type discoverWebsiteSectionsRequest struct {
	URL string `json:"url"`
}

// DiscoverWebsiteSections handles POST /v1/rag/website/discover-sections.
// @Summary Discover the sections of a website
// @Description Crawls a site's links via the configured web crawl provider and groups them into sections (path clusters) and individual pages, so a website knowledge source can be scoped to a set of URLs.
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
	if h.web == nil {
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

	// discoveryPageLimit caps how many URLs the map enumerates — enough to
	// surface the site's sections without a full crawl.
	const discoveryPageLimit = 300
	urls, err := h.web.Map(ctx, webcrawl.MapRequest{URL: target, Limit: discoveryPageLimit})
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "website discovery: map failed", "url", target, "error", err)
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to crawl site: " + err.Error()})
		return
	}

	result, err := website.GroupLinks(target, urls, 0)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}
