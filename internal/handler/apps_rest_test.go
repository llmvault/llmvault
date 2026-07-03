package handler_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestAppsCreateAndGet(t *testing.T) {
	h := newAppsRESTHarness(t)

	rr := h.do(t, "POST", "/v1/apps", map[string]string{
		"channel_id":  h.channel.ID.String(),
		"sheet_id":    h.sheet.ID.String(),
		"name":        "Task Tracker",
		"description": "manage tasks",
		"icon":        "📋",
	}, true)
	if rr.Code != 201 {
		t.Fatalf("create status = %d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		ID     string `json:"id"`
		Slug   string `json:"slug"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Slug != "task-tracker" || created.Status != model.AppStatusDraft {
		t.Fatalf("created = %+v", created)
	}

	// The creating user is attributed.
	var app model.App
	if err := h.db.Where("id = ?", created.ID).First(&app).Error; err != nil {
		t.Fatalf("load app: %v", err)
	}
	if app.CreatedByUserID == nil || *app.CreatedByUserID != h.user.ID {
		t.Fatalf("created_by_user_id = %v", app.CreatedByUserID)
	}

	get := h.do(t, "GET", "/v1/apps/"+created.ID, nil, true)
	if get.Code != 200 {
		t.Fatalf("get status = %d body=%s", get.Code, get.Body.String())
	}
	var detail struct {
		App struct {
			ID string `json:"id"`
		} `json:"app"`
		Versions []any `json:"versions"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if detail.App.ID != created.ID || len(detail.Versions) != 0 {
		t.Fatalf("detail = %+v", detail)
	}
}

func TestAppsCreateDuplicateSlugConflicts(t *testing.T) {
	h := newAppsRESTHarness(t)
	h.createAppViaAPI(t, "Reports")

	rr := h.do(t, "POST", "/v1/apps", map[string]string{
		"channel_id": h.channel.ID.String(),
		"sheet_id":   h.sheet.ID.String(),
		"name":       "reports",
	}, true)
	if rr.Code != 409 {
		t.Fatalf("duplicate slug status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAppsCreateSheetNotInChannelRejected(t *testing.T) {
	h := newAppsRESTHarness(t)

	rr := h.do(t, "POST", "/v1/apps", map[string]string{
		"channel_id": h.channel.ID.String(),
		"sheet_id":   h.crossWire.ID.String(), // lives in otherChan
		"name":       "Cross Wire",
	}, true)
	if rr.Code != 400 {
		t.Fatalf("cross-channel sheet status = %d body=%s", rr.Code, rr.Body.String())
	}

	// A channel the caller cannot address at all 404s.
	rr = h.do(t, "POST", "/v1/apps", map[string]string{
		"channel_id": uuid.NewString(),
		"sheet_id":   h.sheet.ID.String(),
		"name":       "Ghost Channel",
	}, true)
	if rr.Code != 404 {
		t.Fatalf("missing channel status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAppsListAndArchive(t *testing.T) {
	h := newAppsRESTHarness(t)
	appID := h.createAppViaAPI(t, "Listable")

	list := h.do(t, "GET", "/v1/apps?channel_id="+h.channel.ID.String(), nil, true)
	if list.Code != 200 {
		t.Fatalf("list status = %d body=%s", list.Code, list.Body.String())
	}
	var listed struct {
		Apps []struct {
			ID string `json:"id"`
		} `json:"apps"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed.Apps) != 1 || listed.Apps[0].ID != appID {
		t.Fatalf("listed = %+v", listed)
	}

	// Missing channel_id is a caller error.
	if rr := h.do(t, "GET", "/v1/apps", nil, true); rr.Code != 400 {
		t.Fatalf("list without channel_id status = %d", rr.Code)
	}

	// Archive stops the (deployed) sandbox best-effort and 404s afterwards.
	h.markDeployed(t, appID)
	if rr := h.do(t, "DELETE", "/v1/apps/"+appID, nil, true); rr.Code != 204 {
		t.Fatalf("archive status = %d body=%s", rr.Code, rr.Body.String())
	}
	if len(h.provider.stopped) != 1 || h.provider.stopped[0] != "ext-launch" {
		t.Fatalf("stopped sandboxes = %v", h.provider.stopped)
	}
	if rr := h.do(t, "GET", "/v1/apps/"+appID, nil, true); rr.Code != 404 {
		t.Fatalf("archived get status = %d", rr.Code)
	}
}
