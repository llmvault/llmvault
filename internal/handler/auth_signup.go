package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/model"
)

// createUserDefaultOrg is the single source of truth for "this user is signing
// up — give them their default workspace." All three signup entry points
// (password Register, OTPVerify, OAuth findOrCreateUser) call this so that any
// bootstrap side effects (notably the welcome credit grant) live in one place
// and stay consistent.
//
// Side effects, all atomic with the caller's transaction:
//  1. Create the org named "<user.Name>'s Workspace" (PlanSlug defaults to "free").
//  2. Insert an owner OrgMembership for user → org.
//  3. Force a first team named teamName, with the user as an owner team member.
//  4. Provision that team: its own default Hivy agent + its #general channel
//     (user is a #general owner member). There is no team-less #general and no
//     floating org-level default agent anymore.
//  5. If a free plan row exists with WelcomeCredits > 0, grant that amount to
//     the new org as a permanent (non-expiring) credit, refType="signup",
//     refID=user.ID. The unique credit-ledger index keys off (org, reason,
//     ref_type, ref_id), so the grant is naturally idempotent if signup is
//     ever retried for the same user.
//
// teamName must be non-empty — signup forces a user-provided first team name.
// Callers without an interactive name (OTP/OAuth) pass a derived default.
//
// Welcome credits are intentionally only granted here. Subsequent orgs the
// user creates via /v1/orgs do not receive them — that handler stays
// untouched and goes through its own (un-helped) path.
func createUserDefaultOrg(ctx context.Context, tx *gorm.DB, credits *billing.CreditsService, user *model.User, teamName string) (model.Org, error) {
	teamName = normalizeTeamName(teamName)
	if teamName == "" {
		return model.Org{}, fmt.Errorf("team name is required")
	}
	org := model.Org{
		Name: fmt.Sprintf("%s's Workspace", user.Name),
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
	if _, err := provisionFirstTeam(ctx, tx, org.ID, user.ID, teamName); err != nil {
		return org, err
	}
	return org, nil
}

// provisionFirstTeam creates a named team, adds createdByUserID as its owner
// member, and provisions the team's defaults (Hivy + #general). Shared by the
// signup bootstrap and OrgHandler.Create so every org is born with at least one
// self-sufficient team.
func provisionFirstTeam(ctx context.Context, tx *gorm.DB, orgID, createdByUserID uuid.UUID, teamName string) (model.Team, error) {
	teamName = normalizeTeamName(teamName)
	if teamName == "" {
		teamName = defaultTeamName
	}
	createdBy := createdByUserID
	team := model.Team{
		OrgID:     orgID,
		Name:      teamName,
		CreatedBy: &createdBy,
	}
	if err := tx.WithContext(ctx).Create(&team).Error; err != nil {
		return model.Team{}, fmt.Errorf("creating first team: %w", err)
	}
	teamMember := model.TeamMember{
		OrgID:  orgID,
		TeamID: team.ID,
		UserID: createdByUserID,
		Role:   "owner",
	}
	if err := tx.WithContext(ctx).Create(&teamMember).Error; err != nil {
		return model.Team{}, fmt.Errorf("creating first team member: %w", err)
	}
	if _, _, err := provisionTeamDefaults(ctx, tx, orgID, team.ID, createdByUserID); err != nil {
		return model.Team{}, err
	}
	return team, nil
}

// firstTeamNameForUser derives a sensible first-team name for non-interactive
// signup flows (OTP, OAuth) that cannot prompt for one.
func firstTeamNameForUser(user *model.User) string {
	if name := normalizeTeamName(user.Name); name != "" {
		return name + "'s Team"
	}
	return defaultTeamName
}

// grantWelcomeCredits looks up the free plan and, if its WelcomeCredits is
// configured (> 0), writes the grant entry on the supplied transaction.
//
// A missing free plan row is treated as "welcome grants are not configured"
// and is not an error — self-hosted deployments without a seeded plan catalog
// still complete signup successfully.
func grantWelcomeCredits(tx *gorm.DB, credits *billing.CreditsService, orgID, userID uuid.UUID) error {
	if credits == nil {
		return nil
	}

	var freePlan model.Plan
	err := tx.Where("slug = ?", billing.FreePlanSlug).First(&freePlan).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil
	case err != nil:
		return fmt.Errorf("loading free plan: %w", err)
	}

	if freePlan.WelcomeCredits <= 0 {
		return nil
	}

	if err := billing.GrantWithTx(
		tx,
		orgID,
		freePlan.WelcomeCredits,
		billing.ReasonWelcomeGrant,
		billing.RefTypeSignup,
		userID.String(),
		nil, // permanent — welcome credits do not expire
	); err != nil {
		return fmt.Errorf("granting welcome credits: %w", err)
	}
	return nil
}
