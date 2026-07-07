package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
)

func seedVisSandbox(t *testing.T, db *gorm.DB, orgID uuid.UUID, agentID *uuid.UUID, external string) model.Sandbox {
	t.Helper()
	sb := model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  &orgID,
		AgentID:                agentID,
		ProviderID:             "daytona",
		ExternalID:             external,
		RuntimeURL:             "https://runtime.example",
		EncryptedRuntimeSecret: []byte{0x1},
		Status:                 "running",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("seed sandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })
	return sb
}

func TestSandboxList_ActorScopedVisibility(t *testing.T) {
	db := connectTestDB(t)
	fx := seedVisFixture(t, db)
	h := handler.NewSandboxHandler(db, nil)

	visSb := seedVisSandbox(t, db, fx.org.ID, &fx.visibleAgent.ID, "vis-sb")
	hidSb := seedVisSandbox(t, db, fx.org.ID, &fx.hiddenAgent.ID, "hid-sb")
	orphanSb := seedVisSandbox(t, db, fx.org.ID, nil, "orphan-sb")

	list := func(c caller) map[string]bool {
		req := httptest.NewRequest(http.MethodGet, "/v1/sandboxes", nil)
		req = c.apply(req, fx.org)
		rr := httptest.NewRecorder()
		h.List(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
		}
		var resp struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		ids := map[string]bool{}
		for _, s := range resp.Data {
			ids[s.ID] = true
		}
		return ids
	}

	member := list(memberCaller(fx))
	if !member[visSb.ID.String()] {
		t.Fatalf("member missing visible sandbox: %v", member)
	}
	if member[hidSb.ID.String()] {
		t.Fatalf("member must not see hidden-agent sandbox: %v", member)
	}
	if member[orphanSb.ID.String()] {
		t.Fatalf("member must not see agent-less sandbox: %v", member)
	}

	for _, c := range []caller{adminCaller(fx), apiKeyCaller()} {
		ids := list(c)
		for _, want := range []uuid.UUID{visSb.ID, hidSb.ID, orphanSb.ID} {
			if !ids[want.String()] {
				t.Fatalf("org-wide caller missing sandbox %s: %v", want, ids)
			}
		}
	}
}

func TestSandboxGet_ActorScopedVisibility(t *testing.T) {
	db := connectTestDB(t)
	fx := seedVisFixture(t, db)
	h := handler.NewSandboxHandler(db, nil)

	visSb := seedVisSandbox(t, db, fx.org.ID, &fx.visibleAgent.ID, "vis-sb")
	hidSb := seedVisSandbox(t, db, fx.org.ID, &fx.hiddenAgent.ID, "hid-sb")

	get := func(c caller, id uuid.UUID) int {
		req := httptest.NewRequest(http.MethodGet, "/v1/sandboxes/"+id.String(), nil)
		req = c.apply(req, fx.org)
		req = withURLParam(req, "id", id.String())
		rr := httptest.NewRecorder()
		h.Get(rr, req)
		return rr.Code
	}

	if code := get(memberCaller(fx), visSb.ID); code != http.StatusOK {
		t.Fatalf("member get visible sandbox = %d, want 200", code)
	}
	if code := get(memberCaller(fx), hidSb.ID); code != http.StatusNotFound {
		t.Fatalf("member get hidden sandbox = %d, want 404", code)
	}
	if code := get(adminCaller(fx), hidSb.ID); code != http.StatusOK {
		t.Fatalf("admin get hidden sandbox = %d, want 200", code)
	}
}
