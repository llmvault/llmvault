package employeesandbox

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type Selector struct {
	DB                     *gorm.DB
	EmployeeRuntimeImage   string
	SpecialistRuntimeImage string
}

func (s Selector) MainRuntime(ctx context.Context, orgID, employeeID uuid.UUID) (*model.Sandbox, error) {
	var sandbox model.Sandbox
	if err := s.baseQuery(ctx, orgID, employeeID).
		Order("created_at DESC").
		First(&sandbox).Error; err != nil {
		return nil, err
	}
	return &sandbox, nil
}

func (s Selector) MainRuntimeByID(ctx context.Context, orgID, employeeID, sandboxID uuid.UUID) (*model.Sandbox, error) {
	var sandbox model.Sandbox
	if err := s.baseQuery(ctx, orgID, employeeID).
		Where("id = ?", sandboxID).
		First(&sandbox).Error; err != nil {
		return nil, err
	}
	return &sandbox, nil
}

func (s Selector) MainRuntimeMap(ctx context.Context, orgID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]model.Sandbox, error) {
	out := make(map[uuid.UUID]model.Sandbox, len(employeeIDs))
	if len(employeeIDs) == 0 {
		return out, nil
	}
	var sandboxes []model.Sandbox
	if err := s.baseQuery(ctx, orgID, uuid.Nil).
		Where("employee_id IN ?", employeeIDs).
		Order("employee_id ASC, created_at DESC").
		Find(&sandboxes).Error; err != nil {
		return out, err
	}
	for _, sandbox := range sandboxes {
		if sandbox.EmployeeID == nil {
			continue
		}
		if _, exists := out[*sandbox.EmployeeID]; exists {
			continue
		}
		out[*sandbox.EmployeeID] = sandbox
	}
	return out, nil
}

func (s Selector) baseQuery(ctx context.Context, orgID, employeeID uuid.UUID) *gorm.DB {
	q := s.DB.WithContext(ctx).
		Where("org_id = ? AND status = ?", orgID, "running")
	if employeeID != uuid.Nil {
		q = q.Where("employee_id = ?", employeeID)
	}
	employeeRepo := imageRepository(s.EmployeeRuntimeImage)
	specialistRepo := imageRepository(s.SpecialistRuntimeImage)
	if employeeRepo != "" {
		q = q.Where("(snapshot_id IS NULL OR snapshot_id = ? OR snapshot_id LIKE ? OR snapshot_id LIKE ?)", employeeRepo, employeeRepo+":%", employeeRepo+"@%")
	} else if specialistRepo != "" {
		q = q.Where("snapshot_id IS NULL OR (snapshot_id <> ? AND snapshot_id NOT LIKE ? AND snapshot_id NOT LIKE ?)", specialistRepo, specialistRepo+":%", specialistRepo+"@%")
	}
	return q
}

func imageRepository(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return ""
	}
	if at := strings.Index(image, "@"); at >= 0 {
		image = image[:at]
	}
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	if lastColon > lastSlash {
		image = image[:lastColon]
	}
	return strings.TrimSpace(image)
}
