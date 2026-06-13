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
- If this is feedback or new information for existing work, continue in the existing Hivy session when one is clearly related.
- If this is a new coding, debugging, review, deployment, or implementation request, start the work in this agent session with the issue/PR URL, repository, branch context, and exact requested outcome.

Before starting work, search previous sessions when needed. Do not create duplicate work for the same GitHub issue or pull request.`
}

func githubCITriggerInstructions() string {
	return `A GitHub check suite completed for a Hivy-created pull request.

Inspect the pull request and check suite result, then choose the right action:

- If an existing Hivy session is already working on this pull request, continue there when possible.
- If no related session exists and checks failed, timed out, were cancelled, or require action, inspect the relevant check runs or logs, then perform follow-up work only when needed.
- If checks passed and no follow-up is needed, comment or update the GitHub thread only when useful; otherwise keep the session concise.

Search previous sessions before deciding. Do not create duplicate work for the same pull request.`
}
