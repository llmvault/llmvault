package handler_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sheets"
)

// seedForeignSheet creates a second sheet in the SAME org under a different team
// the harness user is not a member of — the "team B" resource in the cross-team
// IDOR tests.
func (h *sheetsHarness) seedForeignSheet(t *testing.T) *sheets.SheetStructure {
	t.Helper()
	team := seedTeam(t, h.db, h.org.ID, "foreign-team-"+uuid.NewString()[:8])
	agent := model.Agent{ID: uuid.New(), OrgID: &h.org.ID, TeamID: team.ID, Name: "Foreign Agent " + uuid.NewString(), Model: "test", Status: "active"}
	channel := model.Channel{ID: uuid.New(), OrgID: h.org.ID, Name: "foreign-ch-" + uuid.NewString(), TeamID: team.ID, DefaultAgentID: agent.ID}
	for _, seed := range []any{&agent, &channel} {
		if err := h.db.Create(seed).Error; err != nil {
			t.Fatalf("seed foreign sheet deps: %v", err)
		}
	}
	t.Cleanup(func() {
		h.db.Delete(&model.Channel{}, "id = ?", channel.ID)
		h.db.Delete(&model.Agent{}, "id = ?", agent.ID)
	})
	sheet, err := h.svc.CreateSheet(context.Background(), h.org.ID, sheets.CreateSheetRequest{
		Name:  "Foreign",
		Pages: []sheets.PageSpec{{Name: "Foreign", Fields: []sheets.FieldSpec{{Name: "Data", Type: sheets.FieldTypeText}}}},
	}, sheets.Actor{ChannelID: channel.ID})
	if err != nil {
		t.Fatalf("create foreign sheet: %v", err)
	}
	return sheet
}

// TestSheetFieldMutationBoundToPage proves a field must belong to the addressed
// {sheetID}/{pageID}: another team's field, addressed under a sheet the caller
// legitimately owns, is rejected as if missing (H3 IDOR).
func TestSheetFieldMutationBoundToPage(t *testing.T) {
	h := newSheetsHarness(t)
	foreign := h.seedForeignSheet(t)
	foreignField := foreign.Pages[0].Fields[0].ID
	ownField := h.fields["Score"]

	// DENIED: PATCH another team's field under the caller's own page.
	if rr := h.do(t, &h.org, http.MethodPatch, h.pagePath("/fields/"+foreignField), map[string]any{"name": "pwn"}); rr.Code != http.StatusNotFound {
		t.Fatalf("cross-team field patch status=%d body=%s, want 404", rr.Code, rr.Body.String())
	}
	// DENIED: DELETE another team's field under the caller's own page.
	if rr := h.do(t, &h.org, http.MethodDelete, h.pagePath("/fields/"+foreignField), nil); rr.Code != http.StatusNotFound {
		t.Fatalf("cross-team field delete status=%d body=%s, want 404", rr.Code, rr.Body.String())
	}
	// The foreign field must be untouched.
	var stored model.SheetField
	if err := h.db.First(&stored, "id = ?", foreignField).Error; err != nil {
		t.Fatalf("reload foreign field: %v", err)
	}
	if stored.Name != "Data" || stored.ArchivedAt != nil {
		t.Fatalf("foreign field mutated: name=%q archived=%v", stored.Name, stored.ArchivedAt)
	}

	// ALLOWED: the caller's own field still mutates.
	if rr := h.do(t, &h.org, http.MethodPatch, h.pagePath("/fields/"+ownField), map[string]any{"name": "Score2"}); rr.Code != http.StatusOK {
		t.Fatalf("own field patch status=%d body=%s, want 200", rr.Code, rr.Body.String())
	}
}

// TestSheetViewMutationBoundToPage proves a view must belong to the addressed
// {sheetID}/{pageID}: another team's view, addressed under a sheet the caller
// legitimately owns, is rejected as if missing (H4 IDOR).
func TestSheetViewMutationBoundToPage(t *testing.T) {
	h := newSheetsHarness(t)
	foreign := h.seedForeignSheet(t)

	ctx := context.Background()
	ownView, err := h.svc.CreateView(ctx, h.org.ID, h.page.Page.ID, sheets.ViewSpec{Name: "Own"})
	if err != nil {
		t.Fatalf("create own view: %v", err)
	}
	foreignView, err := h.svc.CreateView(ctx, h.org.ID, foreign.Pages[0].Page.ID, sheets.ViewSpec{Name: "Foreign"})
	if err != nil {
		t.Fatalf("create foreign view: %v", err)
	}

	// DENIED: PATCH another team's view under the caller's own page.
	if rr := h.do(t, &h.org, http.MethodPatch, h.pagePath("/views/"+foreignView.ID.String()), map[string]any{"name": "pwn"}); rr.Code != http.StatusNotFound {
		t.Fatalf("cross-team view patch status=%d body=%s, want 404", rr.Code, rr.Body.String())
	}
	// DENIED: DELETE another team's view under the caller's own page.
	if rr := h.do(t, &h.org, http.MethodDelete, h.pagePath("/views/"+foreignView.ID.String()), nil); rr.Code != http.StatusNotFound {
		t.Fatalf("cross-team view delete status=%d body=%s, want 404", rr.Code, rr.Body.String())
	}
	// The foreign view must be untouched.
	var stored model.SheetView
	if err := h.db.First(&stored, "id = ?", foreignView.ID).Error; err != nil {
		t.Fatalf("reload foreign view: %v", err)
	}
	if stored.Name != "Foreign" || stored.ArchivedAt != nil {
		t.Fatalf("foreign view mutated: name=%q archived=%v", stored.Name, stored.ArchivedAt)
	}

	// ALLOWED: the caller's own view still mutates.
	if rr := h.do(t, &h.org, http.MethodPatch, h.pagePath("/views/"+ownView.ID.String()), map[string]any{"name": "Own2"}); rr.Code != http.StatusOK {
		t.Fatalf("own view patch status=%d body=%s, want 200", rr.Code, rr.Body.String())
	}
}
