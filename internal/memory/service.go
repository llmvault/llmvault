package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/channelagents"
	"github.com/usehivy/hivy/internal/model"
)

type Config struct {
	DB             *gorm.DB
	CacheManager   *cache.Manager
	Embedder       Embedder
	EnqueueEmbed   EnqueueEmbeddingFunc
	EmbeddingModel string
	EmbeddingDim   uint32
}

type Service struct {
	cfg Config
}

func NewService(cfg Config) *Service {
	return &Service{cfg: cfg}
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (*model.AgentMemory, error) {
	if s == nil || s.cfg.DB == nil {
		return nil, fmt.Errorf("memory service is not configured")
	}
	content, err := normalizeContent(req.Content)
	if err != nil {
		return nil, err
	}
	if req.OrgID == uuid.Nil {
		return nil, fmt.Errorf("org_id is required")
	}
	if err := s.validateChannel(ctx, req.OrgID, req.ChannelID); err != nil {
		return nil, err
	}
	mem := &model.AgentMemory{
		ID:                uuid.New(),
		OrgID:             req.OrgID,
		ChannelID:         req.ChannelID,
		Content:           content,
		MemoryFingerprint: strings.TrimSpace(req.MemoryFingerprint),
		Tags:              pq.StringArray(NormalizeTags(req.Tags)),
		Metadata:          normalizeMetadata(req.Metadata),
		EmbeddingModel:    s.embeddingModel(),
		EmbeddingStatus:   model.AgentMemoryEmbeddingPending,
		EmbeddingRevision: 1,
		SourceSessionID:   req.SourceSessionID,
		SourceEventID:     req.SourceEventID,
		CreatedByUserID:   req.CreatedByUserID,
	}
	if err := s.cfg.DB.WithContext(ctx).Create(mem).Error; err != nil {
		return nil, fmt.Errorf("create memory: %w", err)
	}
	if err := s.enqueueEmbed(ctx, mem.ID, mem.EmbeddingRevision); err != nil {
		return mem, err
	}
	return mem, nil
}

func (s *Service) Update(ctx context.Context, req UpdateRequest) (*model.AgentMemory, error) {
	if s == nil || s.cfg.DB == nil {
		return nil, fmt.Errorf("memory service is not configured")
	}
	var mem model.AgentMemory
	err := s.cfg.DB.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", req.ID, req.OrgID).
		First(&mem).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("load memory: %w", err)
	}
	updates := map[string]any{"updated_at": time.Now()}
	reembed := false
	if req.UpdateChannel {
		if err := s.validateChannel(ctx, req.OrgID, req.ChannelID); err != nil {
			return nil, err
		}
		updates["channel_id"] = req.ChannelID
		mem.ChannelID = req.ChannelID
	}
	if req.Content != nil {
		content, err := normalizeContent(*req.Content)
		if err != nil {
			return nil, err
		}
		if content != mem.Content {
			reembed = true
			mem.Content = content
			mem.EmbeddingRevision++
			updates["content"] = content
			updates["embedding_revision"] = mem.EmbeddingRevision
			updates["embedding_status"] = model.AgentMemoryEmbeddingPending
			updates["embedding_model"] = s.embeddingModel()
			updates["embedding_error"] = ""
			updates["embedded_at"] = nil
			updates["embedding"] = gorm.Expr("NULL")
		}
	}
	if req.Tags != nil {
		tags := NormalizeTags(*req.Tags)
		updates["tags"] = pq.StringArray(tags)
		mem.Tags = pq.StringArray(tags)
	}
	if req.Metadata != nil {
		metadata := normalizeMetadata(*req.Metadata)
		updates["metadata"] = metadata
		mem.Metadata = metadata
	}
	if len(updates) > 1 {
		if err := s.cfg.DB.WithContext(ctx).Model(&model.AgentMemory{}).
			Where("id = ? AND org_id = ?", req.ID, req.OrgID).
			Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("update memory: %w", err)
		}
	}
	if reembed {
		if err := s.enqueueEmbed(ctx, mem.ID, mem.EmbeddingRevision); err != nil {
			return &mem, err
		}
	}
	return &mem, nil
}

func (s *Service) Archive(ctx context.Context, req ArchiveRequest) error {
	if s == nil || s.cfg.DB == nil {
		return fmt.Errorf("memory service is not configured")
	}
	now := time.Now()
	res := s.cfg.DB.WithContext(ctx).Model(&model.AgentMemory{}).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", req.ID, req.OrgID).
		Update("archived_at", &now)
	if res.Error != nil {
		return fmt.Errorf("archive memory: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Service) List(ctx context.Context, req ListRequest) ([]model.AgentMemory, error) {
	if s == nil || s.cfg.DB == nil {
		return nil, fmt.Errorf("memory service is not configured")
	}
	q := s.cfg.DB.WithContext(ctx).
		Where("org_id = ? AND archived_at IS NULL", req.OrgID).
		Order("created_at DESC")
	if clause, args := req.Scope.whereSQL(); clause != "" {
		q = q.Where(clause, args...)
	}
	q = s.applyVisibility(q, req.OrgID, req.Visibility)
	if tags := NormalizeTags(req.Tags); len(tags) > 0 {
		q = q.Where("tags && ?", pq.StringArray(tags))
	}
	if !req.NoLimit {
		limit := req.Limit
		if limit <= 0 {
			limit = 50
		} else if limit > 500 {
			limit = 500
		}
		q = q.Limit(limit)
	}
	var rows []model.AgentMemory
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	return rows, nil
}

func (s *Service) MarkEmbeddingReady(ctx context.Context, id uuid.UUID, revision int, vector []float32) (bool, error) {
	if err := validateVector(vector, s.embeddingDim()); err != nil {
		return false, err
	}
	res := s.cfg.DB.WithContext(ctx).Model(&model.AgentMemory{}).
		Where("id = ? AND embedding_revision = ? AND archived_at IS NULL", id, revision).
		Updates(map[string]any{
			"embedding":        gorm.Expr("?::vector", vectorLiteral(vector)),
			"embedding_model":  s.embeddingModel(),
			"embedding_status": model.AgentMemoryEmbeddingReady,
			"embedding_error":  "",
			"embedded_at":      time.Now(),
			"updated_at":       time.Now(),
		})
	if res.Error != nil {
		return false, fmt.Errorf("mark memory embedding ready: %w", res.Error)
	}
	return res.RowsAffected > 0, nil
}

func (s *Service) MarkEmbeddingFailed(ctx context.Context, id uuid.UUID, revision int, cause error) error {
	msg := ""
	if cause != nil {
		msg = strings.TrimSpace(cause.Error())
	}
	if len(msg) > 500 {
		msg = msg[:500]
	}
	return s.cfg.DB.WithContext(ctx).Model(&model.AgentMemory{}).
		Where("id = ? AND embedding_revision = ? AND archived_at IS NULL", id, revision).
		Updates(map[string]any{
			"embedding_status": model.AgentMemoryEmbeddingFailed,
			"embedding_error":  msg,
			"updated_at":       time.Now(),
		}).Error
}

func (s *Service) enqueueEmbed(ctx context.Context, id uuid.UUID, revision int) error {
	if s.cfg.EnqueueEmbed == nil {
		return nil
	}
	return s.cfg.EnqueueEmbed(ctx, id, revision)
}

// applyVisibility constrains q to memories the actor may see: global
// (channel_id IS NULL) rows plus channels usable under the team-based channel
// visibility predicate. It is a no-op when v.Restrict is false (managers and
// API-key callers see every channel).
func (s *Service) applyVisibility(q *gorm.DB, orgID uuid.UUID, v Visibility) *gorm.DB {
	if !v.Restrict {
		return q
	}
	sub := channelagents.VisibleChannelIDsSubquery(s.cfg.DB, orgID, v.UserID)
	return q.Where("channel_id IS NULL OR channel_id IN (?)", sub)
}

func (s *Service) validateChannel(ctx context.Context, orgID uuid.UUID, channelID *uuid.UUID) error {
	if channelID == nil || *channelID == uuid.Nil {
		return nil
	}
	var count int64
	if err := s.cfg.DB.WithContext(ctx).Model(&model.Channel{}).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", *channelID, orgID).
		Count(&count).Error; err != nil {
		return fmt.Errorf("validate memory channel: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("channel_id must belong to org")
	}
	return nil
}

func (s *Service) embeddingModel() string {
	if strings.TrimSpace(s.cfg.EmbeddingModel) != "" {
		return strings.TrimSpace(s.cfg.EmbeddingModel)
	}
	return DefaultEmbeddingModel
}

func (s *Service) embeddingDim() uint32 {
	if s.cfg.EmbeddingDim > 0 {
		return s.cfg.EmbeddingDim
	}
	return DefaultEmbeddingDim
}
