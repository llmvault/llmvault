package orgtier

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

func CheckKnowledgeSourceCapacity(ctx context.Context, db *gorm.DB, orgID uuid.UUID) error {
	if db == nil {
		return fmt.Errorf("knowledge capacity: database is required")
	}
	org, err := loadOrg(ctx, db, orgID)
	if err != nil {
		return err
	}
	used, err := KnowledgeStorageUsed(ctx, db, orgID)
	if err != nil {
		return err
	}
	if used >= LimitsForTier(org.CapacityTier).KnowledgeStorageBytes {
		return ErrKnowledgeStorageLimit
	}
	return nil
}

func KnowledgeStorageUsed(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (int64, error) {
	var used int64
	if err := db.WithContext(ctx).Model(&model.RAGDocumentStorageUsage{}).
		Where("org_id = ?", orgID).
		Select("COALESCE(SUM(storage_bytes), 0)").Scan(&used).Error; err != nil {
		return 0, fmt.Errorf("sum org knowledge storage: %w", err)
	}
	return used, nil
}

type DocumentStorage struct {
	DocumentID string
	Bytes      int64
}

// KnowledgeReservation can restore per-document sizes when the corresponding
// external vector-store write fails.
type KnowledgeReservation struct {
	OrgID       uuid.UUID
	SourceID    uuid.UUID
	DocumentIDs []string
	Previous    []model.RAGDocumentStorageUsage
}

// ReserveDocumentStorage atomically replaces per-document sizes and rejects a
// batch whose projected org total would cross the tier's hard storage limit.
func ReserveDocumentStorage(ctx context.Context, db *gorm.DB, orgID, sourceID uuid.UUID, docs []DocumentStorage) (KnowledgeReservation, error) {
	reservation := KnowledgeReservation{OrgID: orgID, SourceID: sourceID}
	if len(docs) == 0 {
		return reservation, nil
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		org, err := lockOrg(ctx, tx, orgID)
		if err != nil {
			return err
		}
		used, err := KnowledgeStorageUsed(ctx, tx, orgID)
		if err != nil {
			return err
		}
		ids := make([]string, 0, len(docs))
		var replacementBytes int64
		for _, doc := range docs {
			if strings.TrimSpace(doc.DocumentID) == "" || doc.Bytes < 0 {
				return fmt.Errorf("record knowledge storage: invalid document usage")
			}
			ids = append(ids, doc.DocumentID)
			replacementBytes += doc.Bytes
		}
		reservation.DocumentIDs = append([]string(nil), ids...)
		if err := tx.WithContext(ctx).
			Where("org_id = ? AND rag_source_id = ? AND document_id IN ?", orgID, sourceID, ids).
			Find(&reservation.Previous).Error; err != nil {
			return fmt.Errorf("load replaced knowledge storage: %w", err)
		}
		var previousBytes int64
		if err := tx.WithContext(ctx).Model(&model.RAGDocumentStorageUsage{}).
			Where("org_id = ? AND rag_source_id = ? AND document_id IN ?", orgID, sourceID, ids).
			Select("COALESCE(SUM(storage_bytes), 0)").Scan(&previousBytes).Error; err != nil {
			return fmt.Errorf("sum replaced knowledge storage: %w", err)
		}
		if used-previousBytes+replacementBytes > LimitsForTier(org.CapacityTier).KnowledgeStorageBytes {
			return ErrKnowledgeStorageLimit
		}
		now := time.Now()
		rows := make([]model.RAGDocumentStorageUsage, 0, len(docs))
		for _, doc := range docs {
			rows = append(rows, model.RAGDocumentStorageUsage{
				OrgID: orgID, RAGSourceID: sourceID, DocumentID: doc.DocumentID,
				StorageBytes: doc.Bytes, CreatedAt: now, UpdatedAt: now,
			})
		}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "rag_source_id"}, {Name: "document_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"storage_bytes": gorm.Expr("EXCLUDED.storage_bytes"),
				"updated_at":    now,
			}),
		}).Create(&rows).Error; err != nil {
			return fmt.Errorf("record knowledge storage: %w", err)
		}
		return nil
	})
	return reservation, err
}

func (r KnowledgeReservation) Rollback(ctx context.Context, db *gorm.DB) error {
	if db == nil || r.SourceID == uuid.Nil || len(r.DocumentIDs) == 0 {
		return nil
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockOrg(ctx, tx, r.OrgID); err != nil {
			return err
		}
		if err := tx.WithContext(ctx).
			Where("org_id = ? AND rag_source_id = ? AND document_id IN ?", r.OrgID, r.SourceID, r.DocumentIDs).
			Delete(&model.RAGDocumentStorageUsage{}).Error; err != nil {
			return fmt.Errorf("rollback knowledge storage: %w", err)
		}
		if len(r.Previous) > 0 {
			if err := tx.WithContext(ctx).Create(&r.Previous).Error; err != nil {
				return fmt.Errorf("restore knowledge storage: %w", err)
			}
		}
		return nil
	})
}

func DeleteStaleDocumentStorage(ctx context.Context, db *gorm.DB, orgID, sourceID uuid.UUID, keep []string) error {
	q := db.WithContext(ctx).Where("org_id = ? AND rag_source_id = ?", orgID, sourceID)
	if len(keep) > 0 {
		q = q.Where("document_id NOT IN ?", keep)
	}
	if err := q.Delete(&model.RAGDocumentStorageUsage{}).Error; err != nil {
		return fmt.Errorf("delete stale knowledge storage: %w", err)
	}
	return nil
}
