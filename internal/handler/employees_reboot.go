package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type rebootEmployeeSandboxResponse struct {
	Employee  employeeResponse     `json:"employee"`
	SandboxID string               `json:"sandbox_id"`
	Sync      syncEmployeeResponse `json:"sync"`
}

// RebootSandbox handles POST /v1/employees/{id}/sandbox/reboot.
// @Summary Reboot an employee sandbox
// @Description Restarts the employee sandbox, pushes fresh runtime config, mints fresh proxy credentials, and verifies readiness.
// @Tags employees
// @Produce json
// @Param id path string true "Employee ID"
// @Success 200 {object} rebootEmployeeSandboxResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/sandbox/reboot [post]
func (h *EmployeeHandler) RebootSandbox(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logging.FromContext(ctx)

	if h == nil || h.db == nil || h.orchestrator == nil || h.compileDeps.EncKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "employee sandbox reboot not configured"})
		return
	}

	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}

	employeeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid employee id"})
		return
	}

	var employee model.Employee
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", employeeID, org.ID, "archived").
		First(&employee).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "employee not found"})
			return
		}
		log.ErrorContext(ctx, "load employee for sandbox reboot", "error", err, "employee_id", employeeID, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load employee"})
		return
	}

	if upgrade, ok, err := activeEmployeeSandboxUpgrade(ctx, h.db, org.ID, employeeID); err != nil {
		log.ErrorContext(ctx, "load active employee sandbox upgrade for reboot", "error", err, "employee_id", employeeID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load active upgrade"})
		return
	} else if ok {
		writeEmployeeUpgradeConflict(w, upgrade)
		return
	}

	if h.memoryBanks != nil {
		if err := h.memoryBanks.EnsureOrgBank(ctx, org.ID); err != nil {
			log.ErrorContext(ctx, "ensure memory bank during employee sandbox reboot", "error", err, "employee_id", employeeID, "org_id", org.ID)
			logging.CaptureWithFields(ctx, fmt.Errorf("ensure memory bank during employee sandbox reboot: %w", err), map[string]any{
				"org_id":      org.ID.String(),
				"employee_id": employeeID.String(),
				"bank_id":     hindsight.OrgBankID(org.ID),
			})
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to ensure employee memory bank"})
			return
		}
	}

	sb, err := h.ensureEmployeeSandbox(ctx, &employee)
	if err != nil {
		log.ErrorContext(ctx, "ensure employee sandbox during reboot", "error", err, "employee_id", employeeID)
		logging.Capture(ctx, fmt.Errorf("ensure employee sandbox during reboot: %w", err))
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to provision employee sandbox"})
		return
	}

	if err := h.orchestrator.RestartEmployeeSandbox(ctx, sb); err != nil {
		log.ErrorContext(ctx, "restart employee sandbox", "error", err, "employee_id", employeeID, "sandbox_id", sb.ID)
		logging.Capture(ctx, fmt.Errorf("restart employee sandbox: %w", err))
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to restart employee sandbox"})
		return
	}

	syncResp, err := h.runEmployeeSync(ctx, &employee, sb)
	if err != nil {
		log.ErrorContext(ctx, "sync employee after sandbox reboot", "error", err, "employee_id", employeeID, "sandbox_id", sb.ID)
		logging.Capture(ctx, fmt.Errorf("sync employee after sandbox reboot: %w", err))
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "employee sandbox restarted, but rejected sync"})
		return
	}

	log.InfoContext(ctx, "employee sandbox rebooted and synced", "employee_id", employee.ID, "sandbox_id", sb.ID)
	writeJSON(w, http.StatusOK, rebootEmployeeSandboxResponse{
		Employee:  toEmployeeResponse(employee),
		SandboxID: sb.ID.String(),
		Sync:      toSyncResponseDTO(syncResp),
	})
}
