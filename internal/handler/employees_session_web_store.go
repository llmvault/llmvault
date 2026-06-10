package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/employeesandbox"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *EmployeeHandler) webSessionForMessage(ctx context.Context, orgID, employeeID uuid.UUID, rawSessionID, text string) (model.EmployeeSession, string, bool, error) {
	if strings.TrimSpace(rawSessionID) != "" {
		sessionID, err := uuid.Parse(rawSessionID)
		if err != nil {
			return model.EmployeeSession{}, "", false, errBadSessionID
		}
		var session model.EmployeeSession
		err = h.db.WithContext(ctx).
			Where("id = ? AND org_id = ? AND employee_id = ?", sessionID, orgID, employeeID).
			First(&session).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.EmployeeSession{}, "", false, errSessionNotFound
			}
			return model.EmployeeSession{}, "", false, err
		}
		if session.Source != webSessionSource {
			return model.EmployeeSession{}, "", false, errSessionNotWeb
		}
		if session.Status != "active" {
			return model.EmployeeSession{}, "", false, errSessionNotActive
		}
		return session, runtimeConversationInput(session.RuntimeConversationID), false, nil
	}

	sandbox, err := employeesandbox.Selector{DB: h.db}.MainRuntime(ctx, orgID, employeeID)
	if err != nil {
		return model.EmployeeSession{}, "", false, fmt.Errorf("load employee sandbox: %w", err)
	}
	conversationID := "web-" + uuid.NewString()
	session := model.EmployeeSession{
		OrgID:                 orgID,
		EmployeeID:            employeeID,
		SandboxID:             sandbox.ID,
		RuntimeConversationID: "http-" + conversationID,
		Source:                webSessionSource,
		SourceResourceKey:     conversationID,
		Status:                "active",
		Name:                  webSessionName(text),
		IntegrationScopes:     model.JSON{},
	}
	if err := h.db.WithContext(ctx).Create(&session).Error; err != nil {
		return model.EmployeeSession{}, "", false, fmt.Errorf("create web session: %w", err)
	}
	return session, conversationID, true, nil
}

func (h *EmployeeHandler) runtimeClientForSession(ctx context.Context, session model.EmployeeSession) (*employeeruntime.Client, error) {
	if h.orchestrator == nil {
		return nil, fmt.Errorf("employee runtime is not configured")
	}
	var sb model.Sandbox
	if err := h.db.WithContext(ctx).Where("id = ?", session.SandboxID).First(&sb).Error; err != nil {
		return nil, fmt.Errorf("load employee session sandbox: %w", err)
	}
	return h.orchestrator.GetRuntimeClient(ctx, &sb)
}

func (h *EmployeeHandler) loadActiveEmployee(ctx context.Context, orgID, employeeID uuid.UUID, w http.ResponseWriter) (model.Employee, bool) {
	var employee model.Employee
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", employeeID, orgID, "archived").
		First(&employee).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
			return employee, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee"})
		return employee, false
	}
	return employee, true
}

func (h *EmployeeHandler) loadWebSession(ctx context.Context, orgID, employeeID, sessionID uuid.UUID, w http.ResponseWriter) (model.EmployeeSession, bool) {
	var session model.EmployeeSession
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND employee_id = ? AND source = ?", sessionID, orgID, employeeID, webSessionSource).
		First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "web session not found"})
			return session, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load web session"})
		return session, false
	}
	return session, true
}

func runtimeConversationInput(runtimeSessionID string) string {
	return strings.TrimPrefix(runtimeSessionID, "http-")
}

func webSessionName(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= 80 {
		return text
	}
	return text[:77] + "..."
}

func webUserDisplayName(ctx context.Context) string {
	if user, ok := middleware.UserFromContext(ctx); ok && user != nil {
		return user.Name
	}
	return ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

var (
	errBadSessionID     = errors.New("invalid session id")
	errSessionNotFound  = errors.New("employee session not found")
	errSessionNotWeb    = errors.New("only web sessions can be continued from the web gateway")
	errSessionNotActive = errors.New("employee session is not active")
)

func writeEmployeeSessionMessageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errBadSessionID), errors.Is(err, errSessionNotWeb), errors.Is(err, errSessionNotActive):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, errSessionNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to prepare employee session"})
	}
}
