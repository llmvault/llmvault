package apps

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/microsandbox/alias"
	"github.com/usehivy/hivy/internal/model"
)

// CreateAppParams describes a new app. The sheet must live in the team:
// an app is bound to exactly one sheet at creation (apps plan philosophy).
type CreateAppParams struct {
	OrgID       uuid.UUID
	TeamID      uuid.UUID
	SheetID     uuid.UUID
	Name        string
	Description string
	Icon        string

	CreatedByUserID  *uuid.UUID
	CreatedByAgentID *uuid.UUID
	SourceSessionID  *uuid.UUID
}

// CreateApp validates the team/sheet binding, derives a unique slug from
// the name (sheets slug normalization; a taken slug is ErrSlugTaken, not
// auto-suffixed — the slug is the app's stable alias stem), generates and
// encrypts the app secret, and persists the app in status draft.
func (s *Service) CreateApp(ctx context.Context, params CreateAppParams) (*model.App, error) {
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return nil, validationErrorf("name is required")
	}
	if params.OrgID == uuid.Nil || params.TeamID == uuid.Nil || params.SheetID == uuid.Nil {
		return nil, validationErrorf("org_id, team_id and sheet_id are required")
	}

	var team model.Team
	err := s.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", params.TeamID, params.OrgID).
		First(&team).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load team: %w", err)
	}

	var sheet model.Sheet
	err = s.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", params.SheetID, params.OrgID).
		First(&sheet).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load sheet: %w", err)
	}
	if sheet.TeamID != team.ID {
		return nil, validationErrorf("sheet does not belong to this team")
	}

	// The slug doubles as the microsandbox alias stem, so it must satisfy the
	// SAME rules the control plane enforces at deploy (internal/microsandbox/alias):
	// reserved/too-short/malformed names, and collisions, are auto-suffixed here
	// so app_create never hands back a slug that hard-fails at publish.
	slug, err := s.resolveAppSlug(ctx, params.OrgID, name)
	if err != nil {
		return nil, err
	}

	secret, err := model.GenerateAppSecret()
	if err != nil {
		return nil, err
	}
	if s.encKey == nil {
		return nil, fmt.Errorf("apps: sandbox encryption key is not configured")
	}
	encrypted, err := s.encKey.EncryptString(secret)
	if err != nil {
		return nil, fmt.Errorf("encrypt app secret: %w", err)
	}

	app := model.App{
		OrgID:   params.OrgID,
		TeamID:  team.ID,
		SheetID: sheet.ID,
		Slug:    slug,
		// The alias stem defaults to the slug; the alias claim on deploy is the
		// authority on collisions (last-write-wins repoint on the control plane).
		Alias:              slug,
		Name:               name,
		Description:        strings.TrimSpace(params.Description),
		Icon:               strings.TrimSpace(params.Icon),
		EncryptedAppSecret: encrypted,
		Status:             model.AppStatusDraft,
		CreatedByUserID:    params.CreatedByUserID,
		CreatedByAgentID:   params.CreatedByAgentID,
		SourceSessionID:    params.SourceSessionID,
	}
	if err := s.db.WithContext(ctx).Create(&app).Error; err != nil {
		// The partial unique index is the authority under concurrency.
		if isAppSlugUniqueViolation(err) {
			return nil, ErrSlugTaken
		}
		return nil, fmt.Errorf("create app: %w", err)
	}
	return &app, nil
}

// appSlugPattern mirrors the sheets slug normalization convention.
var appSlugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func normalizeAppSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = appSlugPattern.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

// resolveAppSlug derives a slug from name that BOTH passes alias.Validate and
// is free in the org. It first tries the clean normalized slug; if that is
// reserved/too-short/malformed or already taken, it appends a short random
// suffix and retries until a valid, available slug is found ("suffix on
// collision", apps plan). archived slugs don't block reuse (partial index).
func (s *Service) resolveAppSlug(ctx context.Context, orgID uuid.UUID, name string) (string, error) {
	base := normalizeAppSlug(name)
	for attempt := 0; attempt < 16; attempt++ {
		candidate := base
		// Suffix when the clean base is unusable on its own (invalid/reserved/
		// too short) or on any later attempt (collision retry).
		if attempt > 0 || alias.Validate(candidate) != nil {
			candidate = suffixedAppSlug(base)
		}
		if alias.Validate(candidate) != nil {
			continue // structurally unusable; try a fresh suffix
		}
		taken, err := s.appSlugTaken(ctx, orgID, candidate)
		if err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not derive an available slug for %q", name)
}

func (s *Service) appSlugTaken(ctx context.Context, orgID uuid.UUID, slug string) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.App{}).
		Where("org_id = ? AND slug = ? AND archived_at IS NULL", orgID, slug).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("check app slug: %w", err)
	}
	return count > 0, nil
}

// suffixedAppSlug appends a random suffix to base, keeping the result within
// the alias length bound. When base is structurally unusable even with a
// suffix (empty, or a preview-port-prefix like "3000-x"), it falls back to a
// guaranteed-valid "app-<suffix>" so the caller always gets a claimable slug.
func suffixedAppSlug(base string) string {
	suffix := randSlugSuffix()
	stem := strings.Trim(base, "-")
	if max := alias.MaxLen - len(suffix) - 1; len(stem) > max {
		stem = strings.Trim(stem[:max], "-")
	}
	if stem != "" {
		if cand := stem + "-" + suffix; alias.Validate(cand) == nil {
			return cand
		}
	}
	return "app-" + suffix
}

// randSlugSuffix returns 3 lowercase hex chars (crypto/rand). Hex keeps it in
// the [a-z0-9] charset; a short fixed length keeps slugs readable, and the
// collision-retry loop in resolveAppSlug absorbs the higher collision rate.
func randSlugSuffix() string {
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		// rand.Read essentially never fails; a static stem still yields a valid,
		// (very likely) unique slug via the collision-retry loop.
		return "app"
	}
	return hex.EncodeToString(b)[:3]
}

func isAppSlugUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	return strings.Contains(err.Error(), "idx_apps_org_slug_active")
}
