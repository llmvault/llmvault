package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// EmployeeCleanupPayload is the payload for TypeEmployeeCleanup tasks.
type EmployeeCleanupPayload struct {
	EmployeeID         uuid.UUID `json:"employee_id"`
	SandboxExternalIDs []string  `json:"sandbox_external_ids,omitempty"`
}

// NewEmployeeCleanupTask creates a task that cleans up provider sandboxes left behind by an
// employee hard delete. Options are returned separately (see NewWebhookForwardTask).
func NewEmployeeCleanupTask(employeeID uuid.UUID, sandboxExternalIDs ...string) (*asynq.Task, []asynq.Option, error) {
	payload, err := json.Marshal(EmployeeCleanupPayload{
		EmployeeID:         employeeID,
		SandboxExternalIDs: sandboxExternalIDs,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal employee cleanup payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(2 * time.Minute),
	}
	return asynq.NewTask(TypeEmployeeCleanup, payload), opts, nil
}

// SandboxTemplateBuildPayload is the payload for TypeSandboxTemplateBuild tasks.
type SandboxTemplateBuildPayload struct {
	TemplateID uuid.UUID `json:"template_id"`
}

// NewSandboxTemplateBuildTask creates a task that builds a sandbox template
// snapshot. Options are returned separately (see WebhookForwardPayload's NewWebhookForwardTask).
func NewSandboxTemplateBuildTask(templateID uuid.UUID) (*asynq.Task, []asynq.Option, error) {
	payload, err := json.Marshal(SandboxTemplateBuildPayload{TemplateID: templateID})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal sandbox template build payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(2),
		asynq.Timeout(30 * time.Minute),
	}
	return asynq.NewTask(TypeSandboxTemplateBuild, payload), opts, nil
}

// SandboxTemplateRetryBuildPayload is the payload for retry tasks.
type SandboxTemplateRetryBuildPayload struct {
	TemplateID    uuid.UUID `json:"template_id"`
	BuildCommands []string  `json:"build_commands,omitempty"`
}

// NewSandboxTemplateRetryBuildTask creates a task that retries building a
// sandbox template. Options are returned separately (see WebhookForwardPayload's NewWebhookForwardTask).
func NewSandboxTemplateRetryBuildTask(templateID uuid.UUID, buildCommands []string) (*asynq.Task, []asynq.Option, error) {
	payload := SandboxTemplateRetryBuildPayload{
		TemplateID:    templateID,
		BuildCommands: buildCommands,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal sandbox template retry payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(2),
		asynq.Timeout(30 * time.Minute),
	}
	return asynq.NewTask(TypeSandboxTemplateRetryBuild, data), opts, nil
}

// SkillHydratePayload is the payload for TypeSkillHydrate tasks.
type SkillHydratePayload struct {
	SkillID uuid.UUID `json:"skill_id"`
}

// NewSkillHydrateTask creates a task that pulls a git-sourced skill at its
// tracked ref and updates the skill's current bundle. Options are returned separately (see WebhookForwardPayload's NewWebhookForwardTask).
func NewSkillHydrateTask(skillID uuid.UUID) (*asynq.Task, []asynq.Option, error) {
	payload, err := json.Marshal(SkillHydratePayload{SkillID: skillID})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal skill hydrate payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(2 * time.Minute),
	}
	return asynq.NewTask(TypeSkillHydrate, payload), opts, nil
}
