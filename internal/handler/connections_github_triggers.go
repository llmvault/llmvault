package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type githubEmployeeTriggerSpec struct {
	triggerKeys  pq.StringArray
	conditions   model.TriggerMatch
	instructions string
}

func (h *ConnectionHandler) ensureGitHubEmployeeTriggers(ctx context.Context, tx *gorm.DB, orgID uuid.UUID, conn model.Connection, employee *model.Employee) error {
	if employee == nil || !isGitHubProvider(conn.Integration.Provider) {
		return nil
	}
	specs := []githubEmployeeTriggerSpec{
		{
			triggerKeys:  pq.StringArray{"issue_comment.created"},
			conditions:   githubMentionTriggerConditions(),
			instructions: githubMentionTriggerInstructions(),
		},
		{
			triggerKeys:  pq.StringArray{"check_suite.completed"},
			conditions:   githubCITriggerConditions(),
			instructions: githubCITriggerInstructions(),
		},
	}

	for _, spec := range specs {
		if err := upsertGitHubEmployeeTrigger(ctx, tx, orgID, conn.ID, employee.ID, spec); err != nil {
			return err
		}
	}
	return nil
}

func upsertGitHubEmployeeTrigger(ctx context.Context, tx *gorm.DB, orgID, connectionID, employeeID uuid.UUID, spec githubEmployeeTriggerSpec) error {
	conditionsJSON, err := json.Marshal(spec.conditions)
	if err != nil {
		return fmt.Errorf("marshal github trigger conditions: %w", err)
	}
	instructions := strings.TrimSpace(spec.instructions)

	var existing model.EmployeeTrigger
	err = tx.WithContext(ctx).
		Where("org_id = ? AND employee_id = ? AND connection_id = ? AND trigger_type = ?",
			orgID, employeeID, connectionID, "webhook").
		Where("trigger_keys = ? AND conditions = ?::jsonb", spec.triggerKeys, string(conditionsJSON)).
		First(&existing).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("load github trigger %v: %w", spec.triggerKeys, err)
	}
	if err == nil {
		return tx.WithContext(ctx).Model(&existing).Updates(map[string]any{
			"enabled":      true,
			"trigger_keys": spec.triggerKeys,
			"conditions":   model.RawJSON(conditionsJSON),
			"instructions": instructions,
		}).Error
	}

	trigger := model.EmployeeTrigger{
		OrgID:        orgID,
		EmployeeID:   employeeID,
		TriggerType:  "webhook",
		ConnectionID: &connectionID,
		TriggerKeys:  spec.triggerKeys,
		Enabled:      true,
		Conditions:   model.RawJSON(conditionsJSON),
		Instructions: instructions,
	}
	if err := tx.WithContext(ctx).Create(&trigger).Error; err != nil {
		return fmt.Errorf("create github trigger %v: %w", spec.triggerKeys, err)
	}
	return nil
}

func githubMentionTriggerConditions() model.TriggerMatch {
	return model.TriggerMatch{
		Mode: "all",
		Conditions: []model.TriggerCondition{{
			Path:     "comment.body",
			Operator: "matches",
			Value:    `(?i)(^|\s)@usehivy\b`,
		}},
	}
}

func githubCITriggerConditions() model.TriggerMatch {
	return model.TriggerMatch{
		Mode: "all",
		Conditions: []model.TriggerCondition{{
			Path:     "check_suite.pull_requests.0.number",
			Operator: "exists",
		}},
	}
}

func githubMentionTriggerInstructions() string {
	return `A GitHub issue or pull request comment mentioned @usehivy.

Inspect the linked issue or pull request, understand the user's requested work, and choose the right action:

- If this is a simple question, acknowledgement, status request, or small notification, respond directly on GitHub using the GitHub skill.
- If this is feedback or new information for an existing software engineering specialist task, use the recent task list below and call specialist_task_send_message with the matching task_id.
- If this is a new coding, debugging, review, deployment, or implementation request, launch a new software engineering specialist task with the issue/PR URL, repository, branch context, and exact requested outcome.

Before starting specialist work, check the recent software engineering specialist tasks included in this trigger. If the matching task is not listed, search previous sessions before deciding. Do not create duplicate specialist tasks for the same GitHub issue or pull request.`
}

func githubCITriggerInstructions() string {
	return `A GitHub check suite completed for a Hivy-created pull request.

Inspect the pull request and check suite result, then choose the right action:

- If a software engineering specialist task is already working on this pull request, send the check result and any failure details to that task with specialist_task_send_message.
- If no related specialist task exists and checks failed, timed out, were cancelled, or require action, inspect the relevant check runs or logs, then launch a software engineering specialist task only when follow-up work is needed.
- If checks passed and no follow-up is needed, comment or update the GitHub thread only when useful; otherwise keep the session concise.

Use the recent software engineering specialist tasks included in this trigger first. If the matching task is not listed, search previous sessions before deciding. Do not create duplicate specialist tasks for the same pull request.`
}
