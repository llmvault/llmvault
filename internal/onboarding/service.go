package onboarding

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

var (
	ErrNotFound          = errors.New("onboarding: org not found")
	ErrInvalidTransition = errors.New("onboarding: invalid transition")
)

type Service struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Service {
	return &Service{db: db}
}

// TeamCreated advances onboarding only after the first durable team and its
// defaults have been created successfully in the caller's transaction.
func (s *Service) TeamCreated(ctx context.Context, orgID uuid.UUID) error {
	return s.transition(ctx, orgID, model.OnboardingStepTeam, model.OnboardingStepConnections)
}

// Advance moves an org through the two optional onboarding screens. Team
// creation is intentionally excluded so callers cannot skip the mandatory
// first-team step through this endpoint.
func (s *Service) Advance(ctx context.Context, orgID uuid.UUID, next string) error {
	switch next {
	case model.OnboardingStepWelcome:
		return s.transition(ctx, orgID, model.OnboardingStepConnections, next)
	case model.OnboardingStepComplete:
		return s.transition(ctx, orgID, model.OnboardingStepWelcome, next)
	default:
		return ErrInvalidTransition
	}
}

func (s *Service) transition(ctx context.Context, orgID uuid.UUID, current, next string) error {
	result := s.db.WithContext(ctx).
		Model(&model.Org{}).
		Where("id = ? AND onboarding_step = ?", orgID, current).
		Update("onboarding_step", next)
	if result.Error != nil {
		return fmt.Errorf("advance onboarding: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return nil
	}

	var org model.Org
	err := s.db.WithContext(ctx).Select("id", "onboarding_step").Where("id = ?", orgID).First(&org).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load onboarding state: %w", err)
	}
	if org.OnboardingStep == next {
		return nil
	}
	return ErrInvalidTransition
}
