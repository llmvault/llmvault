package onboarding

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/plugins"
	"github.com/usehivy/hivy/internal/teamprovision"
)

var (
	ErrNotFound           = errors.New("onboarding: org not found")
	ErrInvalidTransition  = errors.New("onboarding: invalid transition")
	ErrConnectionRequired = errors.New("onboarding: connection required")
	ErrInvalidTeamState   = errors.New("onboarding: exactly one active team required")
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

// ConnectionCreated installs the matching plugins and grants them to the org's
// sole active team only while the org is on the onboarding connections step.
// Normal connection creation is intentionally left manual.
func (s *Service) ConnectionCreated(ctx context.Context, orgID, userID uuid.UUID, provider string) error {
	var org model.Org
	if err := s.db.WithContext(ctx).
		Select("id", "onboarding_step").
		Where("id = ?", orgID).
		First(&org).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("load onboarding org: %w", err)
	}
	if org.OnboardingStep != model.OnboardingStepConnections {
		return nil
	}

	var teams []model.Team
	if err := s.db.WithContext(ctx).
		Where("org_id = ? AND archived_at IS NULL", orgID).
		Order("created_at ASC, id ASC").
		Limit(2).
		Find(&teams).Error; err != nil {
		return fmt.Errorf("load onboarding team: %w", err)
	}
	if len(teams) != 1 {
		return ErrInvalidTeamState
	}

	pluginIDs, err := plugins.InstallForConnection(ctx, s.db, orgID, userID, provider)
	if err != nil {
		return fmt.Errorf("install onboarding connection plugins: %w", err)
	}
	if len(pluginIDs) == 0 {
		return fmt.Errorf("install onboarding connection plugins: no active plugin for provider %q", provider)
	}
	for _, pluginID := range pluginIDs {
		enabledBy := userID
		if err := teamprovision.EnablePlugin(ctx, s.db, orgID, teams[0].ID, pluginID, &enabledBy); err != nil && !errors.Is(err, teamprovision.ErrPluginAlwaysEnabled) {
			return fmt.Errorf("enable onboarding plugin for team: %w", err)
		}
	}
	return nil
}

// Advance moves an org through onboarding after its mandatory setup work.
// Team creation is intentionally excluded so callers cannot skip the first-team
// step through this endpoint.
func (s *Service) Advance(ctx context.Context, orgID uuid.UUID, next string) error {
	switch next {
	case model.OnboardingStepWelcome:
		var org model.Org
		if err := s.db.WithContext(ctx).
			Select("id", "onboarding_step").
			Where("id = ?", orgID).
			First(&org).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("load onboarding state: %w", err)
		}
		if org.OnboardingStep == next {
			return nil
		}
		if org.OnboardingStep != model.OnboardingStepConnections {
			return ErrInvalidTransition
		}
		var count int64
		if err := s.db.WithContext(ctx).
			Model(&model.Connection{}).
			Where("org_id = ? AND revoked_at IS NULL", orgID).
			Count(&count).Error; err != nil {
			return fmt.Errorf("count onboarding connections: %w", err)
		}
		if count == 0 {
			return ErrConnectionRequired
		}
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
