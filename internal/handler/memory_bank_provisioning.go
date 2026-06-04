package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/logging"
)

type memoryBankProvisioner interface {
	EnsureOrgBank(context.Context, uuid.UUID) error
}

func ensureOrgMemoryBank(ctx context.Context, banks memoryBankProvisioner, orgID uuid.UUID, stage string) {
	if banks == nil || orgID == uuid.Nil {
		return
	}
	if err := banks.EnsureOrgBank(ctx, orgID); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("%s: ensure org memory bank: %w", stage, err), map[string]any{
			"stage":   stage,
			"org_id":  orgID.String(),
			"bank_id": hindsight.OrgBankID(orgID),
		})
	}
}
