package apps

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// CreateAppParams describes a new app. The sheet must live in the channel:
// an app is bound to exactly one sheet at creation (apps plan philosophy).
type CreateAppParams struct {
	OrgID       uuid.UUID
	ChannelID   uuid.UUID
	SheetID     uuid.UUID
	Name        string
	Description string
	Icon        string

	CreatedByUserID  *uuid.UUID
	CreatedByAgentID *uuid.UUID
	SourceSessionID  *uuid.UUID
}

// CreateApp validates the channel/sheet binding, derives a unique slug from
// the name (sheets slug normalization; a taken slug is ErrSlugTaken, not
// auto-suffixed — the slug is the app's stable alias stem), generates and
// encrypts the app secret, and persists the app in status draft.
func (s *Service) CreateApp(ctx context.Context, params CreateAppParams) (*model.App, error) {
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return nil, validationErrorf("name is required")
	}
	if params.OrgID == uuid.Nil || params.ChannelID == uuid.Nil || params.SheetID == uuid.Nil {
		return nil, validationErrorf("org_id, channel_id and sheet_id are required")
	}

	var channel model.Channel
	err := s.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", params.ChannelID, params.OrgID).
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load channel: %w", err)
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
	if sheet.ChannelID != channel.ID {
		return nil, validationErrorf("sheet does not belong to this channel")
	}

	slug := normalizeAppSlug(name)
	if slug == "" {
		slug = "app"
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.App{}).
		Where("org_id = ? AND slug = ? AND archived_at IS NULL", params.OrgID, slug).
		Count(&count).Error; err != nil {
		return nil, fmt.Errorf("check app slug: %w", err)
	}
	if count > 0 {
		return nil, ErrSlugTaken
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
		OrgID:              params.OrgID,
		ChannelID:          channel.ID,
		SheetID:            sheet.ID,
		Slug:               slug,
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

func isAppSlugUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	return strings.Contains(err.Error(), "idx_apps_org_slug_active")
}
