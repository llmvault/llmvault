package apps

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestCreateApp(t *testing.T) {
	h := newAppsTestHarness(t)
	ctx := context.Background()

	app, err := h.svc.CreateApp(ctx, CreateAppParams{
		OrgID:       h.org.ID,
		ChannelID:   h.channel.ID,
		SheetID:     h.sheet.ID,
		Name:        "  Task Tracker!  ",
		Description: " manage tasks ",
		Icon:        "📋",
	})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	if app.Slug != "task-tracker" {
		t.Fatalf("slug = %q, want task-tracker", app.Slug)
	}
	if app.Status != model.AppStatusDraft {
		t.Fatalf("status = %q, want draft", app.Status)
	}
	if app.Name != "Task Tracker!" || app.Description != "manage tasks" {
		t.Fatalf("name/description not trimmed: %q / %q", app.Name, app.Description)
	}
	secret, err := h.encKey.DecryptString(app.EncryptedAppSecret)
	if err != nil {
		t.Fatalf("decrypt app secret: %v", err)
	}
	if !strings.HasPrefix(secret, model.AppSecretPrefix) {
		t.Fatalf("app secret %q missing %q prefix", secret, model.AppSecretPrefix)
	}
}

func TestCreateAppDuplicateSlug(t *testing.T) {
	h := newAppsTestHarness(t)
	ctx := context.Background()

	h.createApp(t, "Reports")
	_, err := h.svc.CreateApp(ctx, CreateAppParams{
		OrgID: h.org.ID, ChannelID: h.channel.ID, SheetID: h.sheet.ID, Name: "REPORTS",
	})
	if !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("duplicate name error = %v, want ErrSlugTaken", err)
	}
}

func TestCreateAppSheetNotInChannel(t *testing.T) {
	h := newAppsTestHarness(t)
	ctx := context.Background()

	_, err := h.svc.CreateApp(ctx, CreateAppParams{
		OrgID: h.org.ID, ChannelID: h.channel.ID, SheetID: h.otherSheet.ID, Name: "Cross Channel",
	})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("cross-channel sheet error = %v, want ValidationError", err)
	}

	_, err = h.svc.CreateApp(ctx, CreateAppParams{
		OrgID: h.org.ID, ChannelID: h.channel.ID, SheetID: uuid.New(), Name: "Ghost Sheet",
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing sheet error = %v, want ErrNotFound", err)
	}
}
