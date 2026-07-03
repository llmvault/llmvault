package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/system"
)

func (r imageDescribeRequest) sessionUUID() (*uuid.UUID, error) {
	raw := strings.TrimSpace(r.SessionID)
	if raw == "" {
		raw = strings.TrimSpace(r.HivySessionID)
	}
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (h *ImageDescribeHandler) imageDescribeUsageSession(w http.ResponseWriter, r *http.Request, req imageDescribeRequest, asset model.AgentAsset, orgID uuid.UUID) (*model.Session, bool) {
	sessionID, err := req.sessionUUID()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, imageDescribeError{Error: "invalid session_id", ErrorCode: "invalid_session_id"})
		return nil, false
	}
	if sessionID == nil {
		return nil, true
	}
	var session model.Session
	if err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND agent_id = ?", *sessionID, orgID, asset.AgentID).
		First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, imageDescribeError{Error: "session not found", ErrorCode: "session_not_found"})
			return nil, false
		}
		writeJSON(w, http.StatusInternalServerError, imageDescribeError{Error: "failed to load session", ErrorCode: "internal_error"})
		return nil, false
	}
	return &session, true
}

func (h *ImageDescribeHandler) trackImageDescribeUsage(ctx context.Context, cred *model.Credential, orgID uuid.UUID, userID string, session *model.Session, asset model.AgentAsset, detailLevel string, res *system.CompletionResult) {
	if res == nil || cred == nil {
		return
	}
	cost, _ := billing.EstimateCostUSD(
		h.registry,
		imageDescribeProviderID,
		imageDescribeCanonicalModel,
		int64(res.Usage.InputTokens),
		int64(res.Usage.OutputTokens),
		int64(res.Usage.CachedTokens),
	)
	payload := buildModelUsagePayload(modelUsageInput{
		Operation:       "image_description",
		OrgID:           orgID,
		ProviderID:      imageDescribeProviderID,
		Model:           imageDescribeCanonicalModel,
		RequestPath:     "/v1/images/describe",
		TokenJTI:        "system:images.describe",
		UserID:          userID,
		Session:         session,
		SandboxID:       asset.SandboxID,
		Credential:      cred,
		InputTokens:     res.Usage.InputTokens,
		OutputTokens:    res.Usage.OutputTokens,
		CachedTokens:    res.Usage.CachedTokens,
		ReasoningTokens: res.Usage.ReasoningTokens,
		Cost:            cost,
		UpstreamStatus:  http.StatusOK,
		Metadata: model.JSON{
			"drive_asset_id": asset.ID.String(),
			"filename":       asset.Filename,
			"content_type":   asset.ContentType,
			"detail_level":   strings.TrimSpace(detailLevel),
			"upstream_model": res.Model,
		},
	})
	dispatchModelUsage(ctx, h.db, h.enqueuer, payload)
}
