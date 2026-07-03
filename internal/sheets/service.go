package sheets

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// positionStep is the gap between appended fractional-index positions.
const positionStep = 1024

// Service is the single write/read path for sheets. REST handlers, MCP
// tools, and the CSV import worker all go through it, so validation, org
// scoping, and operation capture happen exactly once.
type Service struct {
	db             *gorm.DB
	publisher      EventPublisher
	importEnqueuer ImportEnqueuer
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// WithPublisher injects the realtime event publisher. Nil disables publishing.
func (s *Service) WithPublisher(p EventPublisher) *Service {
	s.publisher = p
	return s
}

// WithImportEnqueuer injects the CSV import task enqueuer (Phase 4 plugs the
// Asynq implementation in). Nil means jobs persist but nothing runs them yet.
func (s *Service) WithImportEnqueuer(e ImportEnqueuer) *Service {
	s.importEnqueuer = e
	return s
}

// Actor identifies who performed a mutation for provenance and the
// operation (undo/audit) log. ChannelID is the channel the mutation is scoped
// to — set from the agent's session on the MCP path and from the request on the
// REST path; CreateSheet stamps it onto the new sheet.
type Actor struct {
	AgentID   *uuid.UUID
	UserID    *uuid.UUID
	SessionID *uuid.UUID
	ChannelID uuid.UUID
}

// FieldSpec describes a column to create.
type FieldSpec struct {
	Name    string
	Type    string
	Options model.JSON
}

// PageSpec describes a tab to create inline with a sheet.
type PageSpec struct {
	Name   string
	Fields []FieldSpec
}

type CreateSheetRequest struct {
	Name        string
	Description string
	Icon        string
	Slug        string
	Pages       []PageSpec
}

// PageStructure is a page with its active fields.
type PageStructure struct {
	Page   model.SheetPage
	Fields []model.SheetField
}

// SheetStructure is a sheet with all active pages and their fields.
type SheetStructure struct {
	Sheet model.Sheet
	Pages []PageStructure
}

// CreateSheet creates a sheet with optional inline pages and fields. When no
// pages are given, a default "Page 1" is created so the sheet is immediately
// writable.
func (s *Service) CreateSheet(ctx context.Context, orgID uuid.UUID, req CreateSheetRequest, actor Actor) (*SheetStructure, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "Untitled sheet"
	}
	if actor.ChannelID == uuid.Nil {
		return nil, fmt.Errorf("sheets: a sheet must be created within a channel")
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.Sheet{}).
		Where("org_id = ? AND archived_at IS NULL", orgID).
		Count(&count).Error; err != nil {
		return nil, fmt.Errorf("count org sheets: %w", err)
	}
	if count >= MaxSheetsPerOrg {
		return nil, &LimitError{Limit: "sheets per org", Max: MaxSheetsPerOrg, Actual: int(count) + 1}
	}
	baseSlug := normalizeSlug(req.Slug)
	if baseSlug == "" {
		baseSlug = normalizeSlug(name)
	}
	slug, err := s.availableSheetSlug(ctx, orgID, baseSlug)
	if err != nil {
		return nil, err
	}
	pages := req.Pages
	if len(pages) == 0 {
		pages = []PageSpec{{Name: "Page 1"}}
	}
	if len(pages) > MaxPagesPerSheet {
		return nil, &LimitError{Limit: "pages per sheet", Max: MaxPagesPerSheet, Actual: len(pages)}
	}

	sheet := model.Sheet{
		OrgID:            orgID,
		ChannelID:        actor.ChannelID,
		Slug:             slug,
		Name:             name,
		Description:      strings.TrimSpace(req.Description),
		Icon:             strings.TrimSpace(req.Icon),
		CreatedByAgentID: actor.AgentID,
		CreatedByUserID:  actor.UserID,
		SourceSessionID:  actor.SessionID,
	}
	structure := &SheetStructure{}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&sheet).Error; err != nil {
			return fmt.Errorf("create sheet: %w", err)
		}
		structure.Sheet = sheet
		for i, spec := range pages {
			page, fields, err := s.createPageTx(tx, &sheet, spec, float64((i+1)*positionStep))
			if err != nil {
				return err
			}
			structure.Pages = append(structure.Pages, PageStructure{Page: *page, Fields: fields})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return structure, nil
}

// ListSheets returns one page of a channel's active sheets, newest-updated
// first, with an optional name search. Sheets are channel-scoped, so callers
// pass the channel they have already authorized the requester for. The returned
// cursor is non-empty when more sheets follow; pass it back to continue the walk.
func (s *Service) ListSheets(ctx context.Context, orgID, channelID uuid.UUID, search string, limit int, cursor string) ([]model.Sheet, string, error) {
	pageSize := ClampLimit(limit, QueryLimitREST)
	q := s.db.WithContext(ctx).
		Where("org_id = ? AND channel_id = ? AND archived_at IS NULL", orgID, channelID).
		Order("updated_at DESC, id DESC").
		Limit(pageSize + 1)
	if search = strings.TrimSpace(search); search != "" {
		q = q.Where("name ILIKE ?", "%"+escapeLike(search)+"%")
	}
	if cursor != "" {
		updatedAt, id, err := decodeSheetListCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		q = q.Where("(updated_at, id) < (?, ?)", updatedAt, id)
	}
	var sheets []model.Sheet
	if err := q.Find(&sheets).Error; err != nil {
		return nil, "", fmt.Errorf("list sheets: %w", err)
	}
	next := ""
	if len(sheets) > pageSize {
		sheets = sheets[:pageSize]
		encoded, err := encodeSheetListCursor(&sheets[len(sheets)-1])
		if err != nil {
			return nil, "", err
		}
		next = encoded
	}
	return sheets, next, nil
}

// sheetListCursor is the opaque keyset cursor for ListSheets: the last
// sheet's updated_at and id under the fixed (updated_at DESC, id DESC) order.
type sheetListCursor struct {
	U  string `json:"u"`
	ID string `json:"id"`
}

func encodeSheetListCursor(sheet *model.Sheet) (string, error) {
	raw, err := json.Marshal(sheetListCursor{
		U:  sheet.UpdatedAt.UTC().Format(time.RFC3339Nano),
		ID: sheet.ID.String(),
	})
	if err != nil {
		return "", fmt.Errorf("encode sheet list cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeSheetListCursor(cursor string) (time.Time, uuid.UUID, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}
	var payload sheetListCursor
	if err := json.Unmarshal(raw, &payload); err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, payload.U)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}
	id, err := uuid.Parse(payload.ID)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}
	return updatedAt, id, nil
}
