package handler

import (
	"context"
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

type employeeSandboxSummary struct {
	ID           string  `json:"id"`
	Status       string  `json:"status"`
	ExternalID   string  `json:"external_id"`
	ErrorMessage *string `json:"error_message,omitempty"`
	LastActiveAt *string `json:"last_active_at,omitempty"`
	CreatedAt    string  `json:"created_at"`
	snapshotID   *string
}

type employeeListItem struct {
	employeeResponse
	UpgradeAvailable bool                    `json:"upgrade_available"`
	Sandbox          *employeeSandboxSummary `json:"sandbox,omitempty"`
}

// @Summary List AI employees
// @Description Returns all employees in the org with skills (metadata only — no bundle content),
// @Description triggers, and the latest sandbox row.
// @Tags employees
// @Produce json
// @Param status query string false "Filter by status (draft, active, archived)"
// @Param limit query int false "Page size (default 50, max 100)"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[employeeListItem]
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees [get]
func (h *EmployeeHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.WithContext(r.Context()).
		Preload("Credential").
		Where("employees.org_id = ?", org.ID)

	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("employees.status = ?", status)
	}

	q = applyPagination(q, cursor, limit)

	var agents []model.Employee
	if err := q.Find(&agents).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list employees"})
		return
	}

	hasMore := len(agents) > limit
	if hasMore {
		agents = agents[:limit]
	}
	h.ensureReturnedEmployeeMemoryBanks(r.Context(), org.ID, agents)

	agentIDs := make([]uuid.UUID, len(agents))
	for i, a := range agents {
		agentIDs[i] = a.ID
	}

	triggers := h.loadEmployeeTriggers(agentIDs...)
	skills := h.loadEmployeeSkills(agentIDs...)
	sandboxes := h.loadMainEmployeeRuntimeSandboxSummaries(r.Context(), org.ID, agentIDs)
	currentSnapshotID := h.currentEmployeeSandboxSnapshotID()

	items := make([]employeeListItem, len(agents))
	for i, a := range agents {
		base := toEmployeeResponse(a)
		base.Triggers = triggers[a.ID]
		base.AttachedSkills = h.markEmployeeSkillLocks(r.Context(), org.ID, &a, skills[a.ID])
		items[i] = employeeListItem{
			employeeResponse: base,
			UpgradeAvailable: employeeSandboxUpgradeAvailable(sandboxes[a.ID], currentSnapshotID),
			Sandbox:          sandboxes[a.ID],
		}
	}

	result := paginatedResponse[employeeListItem]{Data: items, HasMore: hasMore}
	if hasMore {
		last := agents[len(agents)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /v1/employees/{id}.
// @Summary Get an AI employee
// @Description Returns one employee in the org with skills (metadata only — no bundle content),
// @Description triggers, and the latest sandbox row.
// @Tags employees
// @Produce json
// @Param id path string true "Employee agent ID"
// @Success 200 {object} employeeListItem
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id} [get]
func (h *EmployeeHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee id"})
		return
	}

	var agent model.Employee
	if err := h.db.WithContext(r.Context()).
		Preload("Credential").
		Where("employees.id = ? AND employees.org_id = ? AND employees.status <> ?", agentID, org.ID, "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get employee"})
		return
	}
	h.ensureReturnedEmployeeMemoryBanks(r.Context(), org.ID, []model.Employee{agent})

	base := toEmployeeResponse(agent)
	base.Triggers = h.loadEmployeeTriggers(agent.ID)[agent.ID]
	base.AttachedSkills = h.markEmployeeSkillLocks(r.Context(), org.ID, &agent, h.loadEmployeeSkills(agent.ID)[agent.ID])
	sandbox := h.loadMainEmployeeRuntimeSandboxSummaries(r.Context(), org.ID, []uuid.UUID{agent.ID})[agent.ID]
	currentSnapshotID := h.currentEmployeeSandboxSnapshotID()

	writeJSON(w, http.StatusOK, employeeListItem{
		employeeResponse: base,
		UpgradeAvailable: employeeSandboxUpgradeAvailable(sandbox, currentSnapshotID),
		Sandbox:          sandbox,
	})
}

func (h *EmployeeHandler) ensureReturnedEmployeeMemoryBanks(ctx context.Context, orgID uuid.UUID, agents []model.Employee) {
	if h == nil || h.memoryBanks == nil || orgID == uuid.Nil || len(agents) == 0 {
		return
	}
	if err := h.memoryBanks.EnsureOrgBank(ctx, orgID); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("ensure returned employee memory bank: %w", err), map[string]any{
			"org_id":         orgID.String(),
			"employee_count": len(agents),
			"bank_id":        hindsight.OrgBankID(orgID),
		})
	}
}

func (h *EmployeeHandler) currentEmployeeSandboxSnapshotID() string {
	if h == nil || h.compileDeps.Cfg == nil {
		return ""
	}
	return h.compileDeps.Cfg.SandboxesRuntimeBaseImage
}

func (h *EmployeeHandler) employeeListItem(ctx context.Context, orgID uuid.UUID, agent model.Employee) employeeListItem {
	base := toEmployeeResponse(agent)
	base.Triggers = h.loadEmployeeTriggers(agent.ID)[agent.ID]
	base.AttachedSkills = h.markEmployeeSkillLocks(ctx, orgID, &agent, h.loadEmployeeSkills(agent.ID)[agent.ID])
	sandbox := h.loadMainEmployeeRuntimeSandboxSummaries(ctx, orgID, []uuid.UUID{agent.ID})[agent.ID]
	return employeeListItem{
		employeeResponse: base,
		UpgradeAvailable: employeeSandboxUpgradeAvailable(sandbox, h.currentEmployeeSandboxSnapshotID()),
		Sandbox:          sandbox,
	}
}

func employeeSandboxUpgradeAvailable(summary *employeeSandboxSummary, currentSnapshotID string) bool {
	if summary == nil {
		return false
	}
	if summary.snapshotID == nil || *summary.snapshotID == "" {
		return currentSnapshotID != ""
	}
	return *summary.snapshotID != currentSnapshotID
}

func (h *EmployeeHandler) loadMainEmployeeRuntimeSandboxSummaries(ctx context.Context, orgID uuid.UUID, agentIDs []uuid.UUID) map[uuid.UUID]*employeeSandboxSummary {
	employeeImage := ""
	if h != nil && h.compileDeps.Cfg != nil {
		employeeImage = h.compileDeps.Cfg.SandboxesRuntimeBaseImage
	}
	return loadMainEmployeeRuntimeSandboxPerAgent(
		ctx,
		h.db,
		orgID,
		agentIDs,
		employeeImage,
	)
}
