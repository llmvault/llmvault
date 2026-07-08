package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestAppsInternalRowsRoundTrip drives the full row lifecycle through the
// internal app API: insert → query → update → delete.
func TestAppsInternalRowsRoundTrip(t *testing.T) {
	h := newAppsHarness(t)
	titleField := h.fields["Title"]
	scoreField := h.fields["Score"]
	rowsPath := h.pagePath(h.page.Page.ID, "/rows")

	resp := h.do(t, http.MethodPost, rowsPath, h.secret, "", map[string]any{
		"rows": []map[string]any{
			{"data": map[string]any{titleField: "First", scoreField: 10}},
			{"data": map[string]any{titleField: "Second", scoreField: 20}},
		},
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("insert status=%d body=%s", resp.Code, resp.Body.String())
	}
	inserted := decodeAppRows(t, resp.Body.Bytes())
	if len(inserted) != 2 {
		t.Fatalf("expected 2 inserted rows, got %d", len(inserted))
	}

	resp = h.do(t, http.MethodPost, h.pagePath(h.page.Page.ID, "/rows/query"), h.secret, "", map[string]any{
		"filter": map[string]any{"field": titleField, "op": "eq", "value": "Second"},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("query status=%d body=%s", resp.Code, resp.Body.String())
	}
	queried := decodeAppRows(t, resp.Body.Bytes())
	if len(queried) != 1 || queried[0].Data[titleField] != "Second" {
		t.Fatalf("unexpected query result: %+v", queried)
	}

	resp = h.do(t, http.MethodPatch, rowsPath, h.secret, "", map[string]any{
		"rows": []map[string]any{{"id": inserted[0].ID, "data": map[string]any{titleField: "Renamed"}}},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", resp.Code, resp.Body.String())
	}
	updated := decodeAppRows(t, resp.Body.Bytes())
	if len(updated) != 1 || updated[0].Data[titleField] != "Renamed" {
		t.Fatalf("unexpected update result: %+v", updated)
	}
	// Partial merge: the untouched score cell survives the title update.
	if got, ok := updated[0].Data[scoreField].(float64); !ok || got != 10 {
		t.Fatalf("score cell lost on partial update: %+v", updated[0].Data)
	}

	resp = h.do(t, http.MethodDelete, rowsPath, h.secret, "", map[string]any{
		"ids": []string{inserted[1].ID},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", resp.Code, resp.Body.String())
	}
	var archived struct {
		Archived int64 `json:"archived"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &archived); err != nil {
		t.Fatalf("decode delete response: %v", err)
	}
	if archived.Archived != 1 {
		t.Fatalf("expected 1 archived row, got %d", archived.Archived)
	}

	resp = h.do(t, http.MethodPost, h.pagePath(h.page.Page.ID, "/rows/query"), h.secret, "", map[string]any{})
	if resp.Code != http.StatusOK {
		t.Fatalf("final query status=%d body=%s", resp.Code, resp.Body.String())
	}
	if remaining := decodeAppRows(t, resp.Body.Bytes()); len(remaining) != 1 {
		t.Fatalf("expected 1 remaining row, got %d", len(remaining))
	}
}

// TestAppsInternalActorAttribution verifies X-Hivy-App-Actor lands on the
// operation log, and that header-less mutations carry the app's agent
// provenance instead.
func TestAppsInternalActorAttribution(t *testing.T) {
	h := newAppsHarness(t)
	titleField := h.fields["Title"]
	rowsPath := h.pagePath(h.page.Page.ID, "/rows")

	t.Run("actor header records actor_user_id", func(t *testing.T) {
		resp := h.do(t, http.MethodPost, rowsPath, h.secret, h.actorToken(t, h.user.ID), map[string]any{
			"rows": []map[string]any{{"data": map[string]any{titleField: "By user"}}},
		})
		if resp.Code != http.StatusCreated {
			t.Fatalf("insert status=%d body=%s", resp.Code, resp.Body.String())
		}
		op := h.lastOperation(t, h.page.Page.ID)
		if op.ActorUserID == nil || *op.ActorUserID != h.user.ID {
			t.Fatalf("operation actor_user_id = %v, want %s", op.ActorUserID, h.user.ID)
		}
		if op.ActorAgentID != nil {
			t.Fatalf("operation actor_agent_id = %v, want nil for user-attributed mutation", op.ActorAgentID)
		}
	})

	t.Run("absent header records the app agent", func(t *testing.T) {
		resp := h.do(t, http.MethodPost, rowsPath, h.secret, "", map[string]any{
			"rows": []map[string]any{{"data": map[string]any{titleField: "By app"}}},
		})
		if resp.Code != http.StatusCreated {
			t.Fatalf("insert status=%d body=%s", resp.Code, resp.Body.String())
		}
		op := h.lastOperation(t, h.page.Page.ID)
		if op.ActorUserID != nil {
			t.Fatalf("operation actor_user_id = %v, want nil for app-only mutation", op.ActorUserID)
		}
		if op.ActorAgentID == nil || *op.ActorAgentID != h.agent.ID {
			t.Fatalf("operation actor_agent_id = %v, want %s", op.ActorAgentID, h.agent.ID)
		}
	})
}

// TestAppsInternalActorRejected proves actor attribution is fail-closed: only a
// PLATFORM-SIGNED token bound to this app and naming a live org member is
// trusted. A forged/unsigned header, a token minted for another app, and a
// signed token for a non-member all 403, and no mutation happens.
func TestAppsInternalActorRejected(t *testing.T) {
	h := newAppsHarness(t)
	titleField := h.fields["Title"]
	rowsPath := h.pagePath(h.page.Page.ID, "/rows")
	body := map[string]any{
		"rows": []map[string]any{{"data": map[string]any{titleField: "Should not exist"}}},
	}

	// A user in another org is a real user but not a member of the app's org.
	otherOrg := createTestOrg(t, h.db)
	outsider := createOutsideUser(t, h, otherOrg.ID)

	for _, tc := range []struct {
		name  string
		actor string
	}{
		// A raw UUID (what forged/legacy app code would forward) is not a signed
		// token, so it is refused rather than trusted.
		{"raw uuid, not a signed token", h.user.ID.String()},
		{"garbage header", "not-a-jwt"},
		// Correctly signed, but minted for a DIFFERENT app.
		{"signed token for another app", h.actorTokenForApp(t, h.user.ID, uuid.NewString())},
		// Correctly signed and bound to this app, but the named user is not a
		// member of the app's org.
		{"signed token for non-member", h.actorToken(t, outsider)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := h.do(t, http.MethodPost, rowsPath, h.secret, tc.actor, body)
			if resp.Code != http.StatusForbidden {
				t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
			}
		})
	}

	var count int64
	if err := h.db.Table("sheet_rows").Where("page_id = ?", h.page.Page.ID).Count(&count).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("rejected actors still inserted %d rows", count)
	}
}
