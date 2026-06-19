package handler_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func TestListAssets_OrgScopeIsolatesOtherOrgs(t *testing.T) {
	h := newAssetsListHarness(t)
	now := time.Now()
	seedAssetRow(t, h.db, h.orgA.ID, h.agentA1, h.sandboxA1, "videos", "a.mp4", "video/mp4", 10, now)
	seedAssetRow(t, h.db, h.orgB.ID, h.agentB1, h.sandboxB1, "videos", "b.mp4", "video/mp4", 10, now)

	rr := h.get(t, "", &h.orgA)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	page := decodeAssetList(t, rr)
	if len(page.Data) != 1 {
		t.Fatalf("expected 1 row, got %d", len(page.Data))
	}
	if page.Data[0]["filename"] != "a.mp4" {
		t.Fatalf("wrong row leaked: %v", page.Data[0])
	}
	if got := page.Data[0]["asset_url"].(string); !strings.HasPrefix(got, "https://api.usehivy.test/v1/assets/preview?path=") {
		t.Fatalf("expected preview asset_url, got %q", got)
	}
}

func TestListAssets_FilterByAgent(t *testing.T) {
	h := newAssetsListHarness(t)
	now := time.Now()
	seedAssetRow(t, h.db, h.orgA.ID, h.agentA1, h.sandboxA1, "videos", "a1.mp4", "video/mp4", 10, now)
	seedAssetRow(t, h.db, h.orgA.ID, h.agentA2, h.sandboxA2, "videos", "a2.mp4", "video/mp4", 10, now)

	rr := h.get(t, "?agent_id="+h.agentA2.String(), &h.orgA)
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rr.Code, rr.Body.String())
	}
	page := decodeAssetList(t, rr)
	if len(page.Data) != 1 || page.Data[0]["filename"] != "a2.mp4" {
		t.Fatalf("filter agent_id failed: %+v", page.Data)
	}
	if page.Data[0]["agent_id"] != h.agentA2.String() {
		t.Fatalf("agent_id field: %v", page.Data[0]["agent_id"])
	}
}

func TestListAssets_IncludesDescription(t *testing.T) {
	h := newAssetsListHarness(t)
	now := time.Now()
	asset := seedAssetRow(t, h.db, h.orgA.ID, h.agentA1, h.sandboxA1, "images", "ui.png", "image/png", 10, now)
	desc := model.RawJSON(`{"category":"product_ui","confidence":0.94}`)
	if err := h.db.Model(&asset).Update("description", desc).Error; err != nil {
		t.Fatalf("update description: %v", err)
	}

	rr := h.get(t, "?agent_id="+h.agentA1.String(), &h.orgA)
	page := decodeAssetList(t, rr)
	if len(page.Data) != 1 {
		t.Fatalf("expected 1 row, got %d", len(page.Data))
	}
	description, ok := page.Data[0]["description"].(map[string]any)
	if !ok || description["category"] != "product_ui" {
		t.Fatalf("description missing from asset response: %+v", page.Data[0])
	}
}

func TestListAssets_FilterByPathAndPrefix(t *testing.T) {
	h := newAssetsListHarness(t)
	now := time.Now()
	seedAssetRow(t, h.db, h.orgA.ID, h.agentA1, h.sandboxA1, "reports/2026", "a.pdf", "application/pdf", 11, now)
	seedAssetRow(t, h.db, h.orgA.ID, h.agentA1, h.sandboxA1, "reports/2025", "b.pdf", "application/pdf", 12, now)
	seedAssetRow(t, h.db, h.orgA.ID, h.agentA1, h.sandboxA1, "", "root.txt", "text/plain", 13, now)

	rr := h.get(t, "?path=reports/2026", &h.orgA)
	page := decodeAssetList(t, rr)
	if len(page.Data) != 1 || page.Data[0]["filename"] != "a.pdf" {
		t.Fatalf("filter path failed: %+v", page.Data)
	}

	rr = h.get(t, "?path_prefix=reports", &h.orgA)
	page = decodeAssetList(t, rr)
	if len(page.Data) != 2 {
		t.Fatalf("filter path_prefix failed: %+v", page.Data)
	}

	rr = h.get(t, "?path=", &h.orgA)
	page = decodeAssetList(t, rr)
	if len(page.Data) != 1 || page.Data[0]["filename"] != "root.txt" {
		t.Fatalf("filter path='' (root) failed: %+v", page.Data)
	}
}

func TestListAssets_SearchExtensionContentTypeAndDate(t *testing.T) {
	h := newAssetsListHarness(t)
	old := time.Date(2026, 5, 31, 9, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 6, 6, 9, 0, 0, 0, time.UTC)
	seedAssetRow(t, h.db, h.orgA.ID, h.agentA1, h.sandboxA1, "metrics", "revenue.csv", "text/csv", 10, recent)
	seedAssetRow(t, h.db, h.orgA.ID, h.agentA1, h.sandboxA1, "notes", "weekly.txt", "text/plain", 10, old)
	seedAssetRow(t, h.db, h.orgA.ID, h.agentA1, h.sandboxA1, "images", "chart.png", "image/png", 10, recent)

	for _, tc := range []struct {
		query string
		want  string
	}{
		{"?q=revenue", "revenue.csv"},
		{"?extension=png", "chart.png"},
		{"?content_type=image/", "chart.png"},
		{"?created_from=2026-06-01&extension=csv", "revenue.csv"},
	} {
		rr := h.get(t, tc.query, &h.orgA)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s got %d: %s", tc.query, rr.Code, rr.Body.String())
		}
		page := decodeAssetList(t, rr)
		if len(page.Data) != 1 || page.Data[0]["filename"] != tc.want {
			t.Fatalf("%s got %+v want %s", tc.query, page.Data, tc.want)
		}
	}
}

func TestListAssets_CombinedFilters(t *testing.T) {
	h := newAssetsListHarness(t)
	now := time.Now()
	seedAssetRow(t, h.db, h.orgA.ID, h.agentA1, h.sandboxA1, "videos", "a1-vid.mp4", "video/mp4", 10, now)
	seedAssetRow(t, h.db, h.orgA.ID, h.agentA1, h.sandboxA1, "exports", "a1-data.csv", "text/csv", 10, now)
	seedAssetRow(t, h.db, h.orgA.ID, h.agentA2, h.sandboxA2, "videos", "a2-vid.mp4", "video/mp4", 10, now)

	q := fmt.Sprintf("?agent_id=%s&path=videos", h.agentA1)
	rr := h.get(t, q, &h.orgA)
	page := decodeAssetList(t, rr)
	if len(page.Data) != 1 || page.Data[0]["filename"] != "a1-vid.mp4" {
		t.Fatalf("combined filter failed: %+v", page.Data)
	}
}

func TestListAssets_ForeignAgentReturnsEmpty(t *testing.T) {
	h := newAssetsListHarness(t)
	now := time.Now()
	seedAssetRow(t, h.db, h.orgB.ID, h.agentB1, h.sandboxB1, "videos", "b.mp4", "video/mp4", 10, now)

	rr := h.get(t, "?agent_id="+h.agentB1.String(), &h.orgA)
	page := decodeAssetList(t, rr)
	if len(page.Data) != 0 {
		t.Fatalf("expected empty (foreign agent), got %d rows", len(page.Data))
	}
}

func TestListAssets_Pagination(t *testing.T) {
	h := newAssetsListHarness(t)
	base := time.Now().Add(-1 * time.Hour)
	for i := range 5 {
		seedAssetRow(t, h.db, h.orgA.ID, h.agentA1, h.sandboxA1, "page", fmt.Sprintf("f%d.txt", i), "text/plain", 10, base.Add(time.Duration(i)*time.Second))
	}

	rr := h.get(t, "?agent_id="+h.agentA1.String()+"&limit=2", &h.orgA)
	page := decodeAssetList(t, rr)
	if len(page.Data) != 2 || !page.HasMore || page.NextCursor == nil {
		t.Fatalf("first page wrong: %+v", page)
	}

	rr = h.get(t,
		fmt.Sprintf("?agent_id=%s&limit=2&cursor=%s", h.agentA1, *page.NextCursor),
		&h.orgA,
	)
	page2 := decodeAssetList(t, rr)
	if len(page2.Data) != 2 || !page2.HasMore {
		t.Fatalf("second page wrong: %+v", page2)
	}
	if page.Data[0]["id"] == page2.Data[0]["id"] {
		t.Fatalf("pagination did not advance")
	}
}

func TestListAssets_RejectsBadFilters(t *testing.T) {
	h := newAssetsListHarness(t)
	for _, q := range []string{
		"?agent_id=not-a-uuid",
		"?limit=0",
		"?limit=abc",
		"?cursor=not-a-number",
		"?created_from=not-a-date",
		"?sort_by=bad",
		"?sort_dir=sideways",
		"?sort_by=filename&cursor=100",
	} {
		rr := h.get(t, q, &h.orgA)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("query %q expected 400, got %d", q, rr.Code)
		}
	}
}

func TestListAssets_MissingOrgContext(t *testing.T) {
	h := newAssetsListHarness(t)
	rr := h.get(t, "", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
