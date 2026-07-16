package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/billing/purchase"
	"github.com/usehivy/hivy/internal/model"
)

// createUserDefaultOrg is the single source of truth for "this user is signing
// up — give them their default workspace." All three signup entry points
// (password Register, OTPVerify, OAuth findOrCreateUser) call this so that any
// bootstrap side effects (notably the welcome credit grant) live in one place
// and stay consistent.
//
// Side effects, all atomic with the caller's transaction:
//  1. Create the org named "<user.Name>'s Workspace".
//  2. Insert an owner OrgMembership for user → org.
//  3. Mark the org as requiring onboarding, beginning with first-team creation.
//  4. Grant the configured welcome credits to
//     the new org as a permanent (non-expiring) credit, refType="signup",
//     refID=user.ID. The unique credit-ledger index keys off (org, reason,
//     ref_type, ref_id), so the grant is naturally idempotent if signup is
//     ever retried for the same user.
//
// Welcome credits are intentionally only granted here. Subsequent orgs the
// user creates via /v1/orgs do not receive them — that handler stays
// untouched and goes through its own (un-helped) path.
func createUserDefaultOrg(ctx context.Context, tx *gorm.DB, credits *billing.CreditsService, user *model.User) (model.Org, error) {
	org := model.Org{
		Name:           fmt.Sprintf("%s's Workspace", user.Name),
		OnboardingStep: model.OnboardingStepTeam,
	}
	if err := tx.Create(&org).Error; err != nil {
		return org, fmt.Errorf("creating org: %w", err)
	}

	membership := model.OrgMembership{
		UserID: user.ID,
		OrgID:  org.ID,
		Role:   "owner",
	}
	if err := tx.Create(&membership).Error; err != nil {
		return org, fmt.Errorf("creating membership: %w", err)
	}

	if err := grantWelcomeCredits(tx, credits, org.ID, user.ID); err != nil {
		return org, err
	}
	return org, nil
}

// provisionFirstTeam creates a named team, adds createdByUserID as its owner
// member, and provisions the team's defaults (Hivy + #general). Shared by the
// signup bootstrap and OrgHandler.Create so every org is born with at least one
// self-sufficient team. createdByUserID may be uuid.Nil for a userless caller
// (system provisioning); the owner member and created_by are then left unset.
func provisionFirstTeam(ctx context.Context, tx *gorm.DB, orgID, createdByUserID uuid.UUID, teamName string) (model.Team, error) {
	teamName = normalizeTeamName(teamName)
	if teamName == "" {
		teamName = defaultTeamName
	}
	team := model.Team{
		OrgID: orgID,
		Name:  teamName,
	}
	if createdByUserID != uuid.Nil {
		createdBy := createdByUserID
		team.CreatedBy = &createdBy
	}
	if err := tx.WithContext(ctx).Create(&team).Error; err != nil {
		return model.Team{}, fmt.Errorf("creating first team: %w", err)
	}
	if createdByUserID != uuid.Nil {
		teamMember := model.TeamMember{
			OrgID:  orgID,
			TeamID: team.ID,
			UserID: createdByUserID,
			Role:   "owner",
		}
		if err := tx.WithContext(ctx).Create(&teamMember).Error; err != nil {
			return model.Team{}, fmt.Errorf("creating first team member: %w", err)
		}
	}
	if _, _, err := provisionTeamDefaults(ctx, tx, orgID, team.ID, createdByUserID); err != nil {
		return model.Team{}, err
	}
	return team, nil
}

// grantWelcomeCredits writes the product's permanent signup grant.
func grantWelcomeCredits(tx *gorm.DB, credits *billing.CreditsService, orgID, userID uuid.UUID) error {
	if credits == nil {
		return nil
	}

	if err := billing.GrantWithTx(
		tx,
		orgID,
		purchase.WelcomeCredits,
		billing.ReasonWelcomeGrant,
		billing.RefTypeSignup,
		userID.String(),
	); err != nil {
		return fmt.Errorf("granting welcome credits: %w", err)
	}
	return nil
}
