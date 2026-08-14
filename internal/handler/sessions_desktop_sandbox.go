package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	sandboxpkg "github.com/usehivy/hivy/internal/sandbox"
)

func (h *SessionHandler) desktopSandbox(r *http.Request, orgID, agentID, userID uuid.UUID) (model.Sandbox, string, error) {
	externalID := desktopSandboxExternalID(agentID, userID)
	var sb model.Sandbox
	err := h.db.WithContext(r.Context()).
		Where("org_id = ? AND agent_id = ? AND provider_id = ? AND external_id = ?", orgID, agentID, sandboxpkg.ProviderDesktop, externalID).
		First(&sb).Error
	if err == nil {
		secret, decryptErr := h.runtimeEncKey.DecryptString(sb.EncryptedRuntimeSecret)
		return sb, secret, decryptErr
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Sandbox{}, "", err
	}
	secret, err := randomDesktopSecret()
	if err != nil {
		return model.Sandbox{}, "", err
	}
	encrypted, err := h.runtimeEncKey.EncryptString(secret)
	if err != nil {
		return model.Sandbox{}, "", err
	}
	now := time.Now().UTC()
	sb = model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  &orgID,
		AgentID:                &agentID,
		ProviderID:             sandboxpkg.ProviderDesktop,
		ExternalID:             externalID,
		RuntimeURL:             "desktop://localhost",
		EncryptedRuntimeSecret: encrypted,
		Status:                 "running",
		LastActiveAt:           &now,
	}
	if err := h.db.WithContext(r.Context()).Create(&sb).Error; err != nil {
		return model.Sandbox{}, "", err
	}
	return sb, secret, nil
}

func (h *SessionHandler) loadDesktopSandbox(w http.ResponseWriter, r *http.Request, orgID, agentID, userID uuid.UUID) (model.Sandbox, bool) {
	var sb model.Sandbox
	err := h.db.WithContext(r.Context()).
		Where("org_id = ? AND agent_id = ? AND provider_id = ? AND external_id = ?", orgID, agentID, sandboxpkg.ProviderDesktop, desktopSandboxExternalID(agentID, userID)).
		First(&sb).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "desktop runtime must be initialized for this agent"})
		return model.Sandbox{}, false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load desktop runtime"})
		return model.Sandbox{}, false
	}
	return sb, true
}

func (h *SessionHandler) authorizeDesktopSession(w http.ResponseWriter, r *http.Request) (model.Session, *uuid.UUID, bool) {
	session, userID, ok := h.authorizeSession(w, r, true)
	if !ok {
		return model.Session{}, nil, false
	}
	if userID == nil || session.SandboxID == nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "session not found"})
		return model.Session{}, nil, false
	}
	var count int64
	err := h.db.WithContext(r.Context()).Model(&model.Sandbox{}).
		Where("id = ? AND org_id = ? AND agent_id = ? AND provider_id = ? AND external_id = ?", *session.SandboxID, session.OrgID, session.AgentID, sandboxpkg.ProviderDesktop, desktopSandboxExternalID(session.AgentID, *userID)).
		Count(&count).Error
	if err != nil || count != 1 {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "session not found"})
		return model.Session{}, nil, false
	}
	return session, userID, true
}

func desktopSandboxExternalID(agentID, userID uuid.UUID) string {
	return "desktop:" + userID.String() + ":" + agentID.String()
}

func randomDesktopSecret() (string, error) {
	raw := make([]byte, 48)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func (h *SessionHandler) deleteDesktopSession(ctx context.Context, sessionID uuid.UUID) {
	if err := h.db.WithContext(ctx).Delete(&model.Session{}, "id = ?", sessionID).Error; err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "clean up failed desktop session", "session_id", sessionID, "error", err)
	}
}
