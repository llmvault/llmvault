package devseed

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// GORMStore persists the development-only onboarding completion.
type GORMStore struct {
	db *gorm.DB
}

func NewGORMStore(db *gorm.DB) *GORMStore {
	return &GORMStore{db: db}
}

func (s *GORMStore) CompleteOnboarding(ctx context.Context, rawOrgID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("development seed database is required")
	}
	orgID, err := uuid.Parse(rawOrgID)
	if err != nil {
		return fmt.Errorf("invalid development organization id: %w", err)
	}
	result := s.db.WithContext(ctx).
		Model(&model.Org{}).
		Where("id = ?", orgID).
		Update("onboarding_step", model.OnboardingStepComplete)
	if result.Error != nil {
		return fmt.Errorf("update development onboarding: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("development organization not found")
	}
	return nil
}
