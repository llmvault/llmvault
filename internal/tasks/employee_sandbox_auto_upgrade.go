package tasks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

const employeeSandboxAutoUpgradeTimeout = 10 * time.Minute

type EmployeeSandboxAutoUpgradePayload struct {
	RuntimeImage string `json:"runtime_image"`
	Limit        int    `json:"limit,omitempty"`
}

type EmployeeSandboxAutoUpgradeHandler struct {
	db          *gorm.DB
	compileDeps employeeruntime.CompileDeps
	enqueuer    enqueue.TaskEnqueuer
}

type outdatedEmployeeSandbox struct {
	OrgID      uuid.UUID
	EmployeeID uuid.UUID
	SandboxID  uuid.UUID
}

func NewEmployeeSandboxAutoUpgradeTask(payload EmployeeSandboxAutoUpgradePayload) (*asynq.Task, []asynq.Option, error) {
	payload.RuntimeImage = strings.TrimSpace(payload.RuntimeImage)
	if payload.RuntimeImage == "" {
		return nil, nil, fmt.Errorf("employee sandbox auto-upgrade runtime image is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal employee sandbox auto-upgrade payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(employeeSandboxAutoUpgradeTimeout),
		asynq.Unique(10 * time.Minute),
		asynq.TaskID("employee-sandbox-auto-upgrade:" + shortHash(payload.RuntimeImage)),
	}
	return asynq.NewTask(TypeEmployeeSandboxAutoUpgrade, body), opts, nil
}

func EnqueueEmployeeSandboxAutoUpgrade(ctx context.Context, enqueuer enqueue.TaskEnqueuer, payload EmployeeSandboxAutoUpgradePayload) error {
	if enqueuer == nil {
		return nil
	}
	task, opts, err := NewEmployeeSandboxAutoUpgradeTask(payload)
	if err != nil {
		return err
	}
	if _, err := enqueuer.EnqueueContext(ctx, task, opts...); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			return nil
		}
		return err
	}
	return nil
}

func NewEmployeeSandboxAutoUpgradeHandler(db *gorm.DB, compileDeps employeeruntime.CompileDeps, enqueuer enqueue.TaskEnqueuer) *EmployeeSandboxAutoUpgradeHandler {
	return &EmployeeSandboxAutoUpgradeHandler{db: db, compileDeps: compileDeps, enqueuer: enqueuer}
}

func (h *EmployeeSandboxAutoUpgradeHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.enqueuer == nil {
		return fmt.Errorf("employee sandbox auto-upgrade handler not configured")
	}
	if h.compileDeps.Cfg == nil {
		return fmt.Errorf("employee sandbox auto-upgrade config not configured")
	}
	var payload EmployeeSandboxAutoUpgradePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal employee sandbox auto-upgrade payload: %w", err)
	}
	payload.RuntimeImage = strings.TrimSpace(payload.RuntimeImage)
	if payload.RuntimeImage == "" {
		return fmt.Errorf("employee sandbox auto-upgrade runtime image is required")
	}
	rows, err := h.loadOutdatedEmployeeSandboxes(ctx, payload.RuntimeImage, payload.Limit)
	if err != nil {
		return err
	}
	var enqueued, skipped int
	for _, row := range rows {
		ok, err := h.enqueueUpgrade(ctx, row)
		if err != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "employee sandbox auto-upgrade enqueue failed",
				"error", err, "employee_id", row.EmployeeID, "sandbox_id", row.SandboxID)
			continue
		}
		if ok {
			enqueued++
		} else {
			skipped++
		}
	}
	logging.FromContext(ctx).InfoContext(ctx, "employee sandbox auto-upgrade sweep complete",
		"runtime_image", payload.RuntimeImage, "candidates", len(rows), "enqueued", enqueued, "skipped", skipped)
	return nil
}

func (h *EmployeeSandboxAutoUpgradeHandler) loadOutdatedEmployeeSandboxes(ctx context.Context, runtimeImage string, limit int) ([]outdatedEmployeeSandbox, error) {
	if limit <= 0 {
		limit = 1000
	}
	specialistRepo := imageRepository(h.compileDeps.Cfg.SandboxesRuntimeSpecialistImage)
	if specialistRepo == "" {
		specialistRepo = defaultSpecialistRepository(runtimeImage)
	}
	args := []any{}
	query := `
WITH latest AS (
	SELECT DISTINCT ON (s.employee_id)
		s.org_id,
		s.employee_id,
		s.id AS sandbox_id,
		s.snapshot_id
	FROM sandboxes s
	JOIN employees e ON e.id = s.employee_id AND e.org_id = s.org_id
	WHERE s.status = 'running'
		AND s.org_id IS NOT NULL
		AND s.employee_id IS NOT NULL
		AND e.status <> 'archived'
`
	if specialistRepo != "" {
		query += `
		AND (s.snapshot_id IS NULL OR (s.snapshot_id <> ? AND s.snapshot_id NOT LIKE ? AND s.snapshot_id NOT LIKE ?))
`
		args = append(args, specialistRepo, specialistRepo+":%", specialistRepo+"@%")
	}
	query += `
	ORDER BY s.employee_id, s.created_at DESC
)
SELECT org_id, employee_id, sandbox_id
FROM latest
WHERE snapshot_id IS NULL OR snapshot_id <> ?
LIMIT ?
`
	args = append(args, runtimeImage, limit)
	var rows []outdatedEmployeeSandbox
	if err := h.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load outdated employee sandboxes: %w", err)
	}
	return rows, nil
}

func (h *EmployeeSandboxAutoUpgradeHandler) enqueueUpgrade(ctx context.Context, row outdatedEmployeeSandbox) (bool, error) {
	if active, ok, err := activeEmployeeSandboxUpgrade(ctx, h.db, row.OrgID, row.EmployeeID); err != nil {
		return false, err
	} else if ok && active != nil {
		return false, nil
	}
	if cleaner, ok := h.enqueuer.(enqueue.TaskCleaner); ok {
		if err := cleaner.DeleteTask(QueueBulk, EmployeeSandboxUpgradeTaskID(row.EmployeeID)); err != nil && !strings.Contains(err.Error(), "not found") {
			if strings.Contains(err.Error(), "active state") {
				return false, nil
			}
			return false, err
		}
	}
	upgrade := model.EmployeeSandboxUpgrade{
		OrgID:        row.OrgID,
		EmployeeID:   row.EmployeeID,
		OldSandboxID: &row.SandboxID,
		Status:       model.EmployeeSandboxUpgradeStatusQueued,
		Phase:        model.EmployeeSandboxUpgradePhaseQueued,
	}
	if err := h.db.WithContext(ctx).Create(&upgrade).Error; err != nil {
		return false, fmt.Errorf("create employee sandbox upgrade: %w", err)
	}
	task, opts, err := NewEmployeeSandboxUpgradeTask(upgrade.ID, row.EmployeeID)
	if err != nil {
		h.markFailed(ctx, &upgrade, model.EmployeeSandboxUpgradePhaseQueued, err.Error())
		return false, err
	}
	if _, err := h.enqueuer.EnqueueContext(ctx, task, opts...); err != nil {
		h.markFailed(ctx, &upgrade, model.EmployeeSandboxUpgradePhaseQueued, err.Error())
		if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func activeEmployeeSandboxUpgrade(ctx context.Context, db *gorm.DB, orgID, employeeID uuid.UUID) (*model.EmployeeSandboxUpgrade, bool, error) {
	var upgrade model.EmployeeSandboxUpgrade
	err := db.WithContext(ctx).
		Where("org_id = ? AND employee_id = ? AND status IN ?", orgID, employeeID, []string{
			model.EmployeeSandboxUpgradeStatusQueued,
			model.EmployeeSandboxUpgradeStatusRunning,
		}).
		Order("created_at DESC").
		First(&upgrade).Error
	if err == nil {
		return &upgrade, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	return nil, false, err
}

func (h *EmployeeSandboxAutoUpgradeHandler) markFailed(ctx context.Context, upgrade *model.EmployeeSandboxUpgrade, phase, message string) {
	now := time.Now().UTC()
	truncated := truncateUpgradeError(message)
	_ = h.db.WithContext(ctx).Model(upgrade).Updates(map[string]any{
		"status":        model.EmployeeSandboxUpgradeStatusFailed,
		"phase":         phase,
		"error_message": truncated,
		"completed_at":  now,
	}).Error
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

func defaultSpecialistRepository(employeeImage string) string {
	repo := imageRepository(employeeImage)
	if repo == "" || strings.HasSuffix(repo, "-specialist") {
		return ""
	}
	return repo + "-specialist"
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:16]
}
