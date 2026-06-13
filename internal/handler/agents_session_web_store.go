package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *AgentHandler) webSessionForMessage(ctx context.Context, orgID, agentID uuid.UUID, rawSessionID, text string) (model.AgentSession, string, bool, error) {
	if strings.TrimSpace(rawSessionID) != "" {
		sessionID, err := uuid.Parse(rawSessionID)
		if err != nil {
			return model.AgentSession{}, "", false, errBadSessionID
		}
		var session model.AgentSession
		err = h.db.WithContext(ctx).
			Where("id = ? AND org_id = ? AND agent_id = ?", sessionID, orgID, agentID).
			First(&session).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.AgentSession{}, "", false, errSessionNotFound
			}
			return model.AgentSession{}, "", false, err
		}
		if session.Source != webSessionSource {
			return model.AgentSession{}, "", false, errSessionNotWeb
		}
		if session.Status != "active" {
			return model.AgentSession{}, "", false, errSessionNotActive
		}
		return session, runtimeConversationInput(session.RuntimeConversationID), false, nil
	}

	sandbox, err := h.mainAgentRuntimeSelector().MainRuntime(ctx, orgID, agentID)
	if err != nil {
		return model.AgentSession{}, "", false, fmt.Errorf("load agent sandbox: %w", err)
	}
	conversationID := "web-" + uuid.NewString()
	session := model.AgentSession{
		OrgID:                 orgID,
		AgentID:               agentID,
		SandboxID:             sandbox.ID,
		RuntimeConversationID: "http-" + conversationID,
		Source:                webSessionSource,
		SourceResourceKey:     conversationID,
		Status:                "active",
		Name:                  webSessionName(text),
		IntegrationScopes:     model.JSON{},
	}
	if err := h.db.WithContext(ctx).Create(&session).Error; err != nil {
		return model.AgentSession{}, "", false, fmt.Errorf("create web session: %w", err)
	}
	return session, conversationID, true, nil
}

func (h *AgentHandler) runtimeClientForSession(ctx context.Context, session model.AgentSession) (*agentruntime.Client, error) {
	client, _, err := h.runtimeClientAndSandboxForSession(ctx, session)
	return client, err
}

func (h *AgentHandler) runtimeClientAndSandboxForSession(ctx context.Context, session model.AgentSession) (*agentruntime.Client, model.Sandbox, error) {
	if h.orchestrator == nil {
		return nil, model.Sandbox{}, fmt.Errorf("agent runtime is not configured")
	}
	var sb model.Sandbox
	if err := h.db.WithContext(ctx).Where("id = ?", session.SandboxID).First(&sb).Error; err != nil {
		return nil, model.Sandbox{}, fmt.Errorf("load agent session sandbox: %w", err)
	}
	client, err := h.orchestrator.GetRuntimeClient(ctx, &sb)
	if err != nil {
		return nil, model.Sandbox{}, err
	}
	return client, sb, nil
}

func (h *AgentHandler) loadActiveAgent(ctx context.Context, orgID, agentID uuid.UUID, w http.ResponseWriter) (model.Agent, bool) {
	var agent model.Agent
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return agent, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load agent"})
		return agent, false
	}
	return agent, true
}

func (h *AgentHandler) loadWebSession(ctx context.Context, orgID, agentID, sessionID uuid.UUID, w http.ResponseWriter) (model.AgentSession, bool) {
	var session model.AgentSession
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND agent_id = ? AND source = ?", sessionID, orgID, agentID, webSessionSource).
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
	errSessionNotFound  = errors.New("agent session not found")
	errSessionNotWeb    = errors.New("only web sessions can be continued from the web gateway")
	errSessionNotActive = errors.New("agent session is not active")
)

func writeAgentSessionMessageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errBadSessionID), errors.Is(err, errSessionNotWeb), errors.Is(err, errSessionNotActive):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, errSessionNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to prepare agent session"})
	}
}
