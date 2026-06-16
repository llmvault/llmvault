package credentials

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func Resolve(
	ctx context.Context,
	db *gorm.DB,
	picker Picker,
	agent *model.Agent,
) (*model.Credential, error) {
	if agent == nil {
		return nil, fmt.Errorf("credentials: Resolve called with nil agent")
	}
	if agent.OrgID == nil {
		return nil, fmt.Errorf("agent %s has no org_id; cannot resolve credential", agent.ID)
	}
	return ResolveForModel(ctx, db, registry.Global(), *agent.OrgID, agent.Model)
}

func ResolveForModel(ctx context.Context, db *gorm.DB, reg *registry.Registry, orgID uuid.UUID, modelID string) (*model.Credential, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, fmt.Errorf("credentials: model is required")
	}
	if db == nil {
		return nil, fmt.Errorf("credentials: db is required")
	}
	if reg == nil {
		reg = registry.Global()
	}
	if err := reg.ValidateCanonicalModel(modelID); err != nil {
		return nil, err
	}
	providers := reg.ProviderPreferenceForModel(modelID)
	if len(providers) == 0 {
		return nil, fmt.Errorf("credentials: model %q has no provider route", modelID)
	}

	creds, err := activeCredentialsForRouting(ctx, db, orgID)
	if err != nil {
		return nil, err
	}
	for _, scope := range []bool{true, false} {
		for _, providerID := range providers {
			for i := range creds {
				if (creds[i].OrgID != nil) != scope {
					continue
				}
				if creds[i].ProviderID != providerID {
					continue
				}
				chosen := creds[i]
				return &chosen, nil
			}
		}
	}
	return nil, fmt.Errorf("credentials: no active org or system credential supports model %q", modelID)
}

func activeCredentialsForRouting(ctx context.Context, db *gorm.DB, orgID uuid.UUID) ([]model.Credential, error) {
	var creds []model.Credential
	if err := db.WithContext(ctx).
		Where("revoked_at IS NULL AND (org_id = ? OR org_id IS NULL)", orgID).
		Order("created_at ASC").
		Find(&creds).Error; err != nil {
		return nil, fmt.Errorf("list active credentials: %w", err)
	}
	return creds, nil
}
