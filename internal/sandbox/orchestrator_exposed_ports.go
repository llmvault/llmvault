package sandbox

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func (o *Orchestrator) loadOrgSandboxExposedPorts(ctx context.Context, orgID uuid.UUID) ([]int, error) {
	var org model.Org
	if err := o.db.WithContext(ctx).
		Select("sandbox_exposed_ports").
		First(&org, "id = ?", orgID).Error; err != nil {
		return nil, fmt.Errorf("loading org sandbox exposed ports: %w", err)
	}
	ports, err := model.NormalizeSandboxExposedPorts(model.SandboxExposedPortsFromInt64Array(org.SandboxExposedPorts))
	if err != nil {
		return nil, fmt.Errorf("loading org sandbox exposed ports: %w", err)
	}
	return ports, nil
}
