package apps

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/microsandbox/alias"
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

// A colliding name no longer errors: app_create auto-suffixes to a fresh,
// valid, available slug ("suffix on collision") instead of ErrSlugTaken.
func TestCreateAppDuplicateSlug(t *testing.T) {
	h := newAppsTestHarness(t)
	ctx := context.Background()

	first := h.createApp(t, "Reports")
	if first.Slug != "reports" {
		t.Fatalf("first slug = %q, want reports", first.Slug)
	}
	second, err := h.svc.CreateApp(ctx, CreateAppParams{
		OrgID: h.org.ID, ChannelID: h.channel.ID, SheetID: h.sheet.ID, Name: "REPORTS",
	})
	if err != nil {
		t.Fatalf("colliding name should suffix, got error: %v", err)
	}
	if second.Slug == first.Slug {
		t.Fatalf("second slug = %q, want a suffixed variant", second.Slug)
	}
	if !strings.HasPrefix(second.Slug, "reports-") {
		t.Fatalf("second slug = %q, want reports-<suffix>", second.Slug)
	}
	if err := alias.Validate(second.Slug); err != nil {
		t.Fatalf("suffixed slug %q rejected by control-plane rules: %v", second.Slug, err)
	}
}

// Names that normalize to a reserved, too-short, or otherwise unclaimable
// alias must be auto-suffixed at app_create into a slug the control plane will
// accept — never stored raw to hard-fail later at deploy.
func TestCreateAppNormalizesInvalidSlug(t *testing.T) {
	h := newAppsTestHarness(t)
	ctx := context.Background()

	cases := []struct {
		name       string
		appName    string
		wantPrefix string
	}{
		{"reserved api", "api", "api-"},
		{"reserved app", "App", "app-"},
		{"reserved apps", "apps", "apps-"},
		{"too short", "ab", "ab-"},
		{"single char", "x", "x-"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, err := h.svc.CreateApp(ctx, CreateAppParams{
				OrgID: h.org.ID, ChannelID: h.channel.ID, SheetID: h.sheet.ID, Name: tc.appName,
			})
			if err != nil {
				t.Fatalf("create %q: %v", tc.appName, err)
			}
			if err := alias.Validate(app.Slug); err != nil {
				t.Fatalf("slug %q for name %q rejected by control-plane rules: %v", app.Slug, tc.appName, err)
			}
			if !strings.HasPrefix(app.Slug, tc.wantPrefix) {
				t.Fatalf("slug = %q, want prefix %q", app.Slug, tc.wantPrefix)
			}
			// The alias stem is stored equal to the slug, so it too must be valid.
			if app.Alias != app.Slug {
				t.Fatalf("alias %q != slug %q", app.Alias, app.Slug)
			}
		})
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
