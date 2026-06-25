package handler

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentsandbox"
	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

const imageDescribeRuntimeMaxBodyBytes = 64 * 1024

func (h *ImageDescribeHandler) DescribeForRuntime(w http.ResponseWriter, r *http.Request) {
	if h.runtimeSecrets == nil {
		writeJSON(w, http.StatusServiceUnavailable, imageDescribeError{Error: "runtime image description unavailable", ErrorCode: "runtime_auth_unavailable"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, imageDescribeError{Error: "invalid agent_id", ErrorCode: "invalid_agent_id"})
		return
	}
	bearer := bearerFromHeader(r.Header.Get("Authorization"))
	if bearer == "" {
		writeJSON(w, http.StatusUnauthorized, imageDescribeError{Error: "missing authorization", ErrorCode: "unauthorized"})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, imageDescribeRuntimeMaxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, imageDescribeError{Error: "invalid request body", ErrorCode: "invalid_request_body"})
		return
	}
	var req imageDescribeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, imageDescribeError{Error: "invalid request body", ErrorCode: "invalid_request_body"})
		return
	}
	assetID, err := uuid.Parse(req.DriveAssetID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, imageDescribeError{Error: "invalid drive_asset_id", ErrorCode: "invalid_drive_asset_id"})
		return
	}

	agent, sandbox, ok := h.runtimeDescribeAgent(w, r, agentID, bearer)
	if !ok {
		return
	}
	org, ok := h.runtimeDescribeOrg(w, r, *agent.OrgID)
	if !ok {
		return
	}
	if !h.runtimeDescribeAssetAllowed(w, r, assetID, *agent.OrgID, agent.ID, sandbox.ID) {
		return
	}

	delegated := r.Clone(r.Context())
	delegated.Body = io.NopCloser(bytes.NewReader(body))
	delegated = middleware.WithOrg(delegated, org)
	delegated = middleware.WithAuthClaims(delegated, &auth.AuthClaims{
		OrgID:  org.ID.String(),
		UserID: "runtime:" + agent.ID.String(),
		Role:   "agent_runtime",
	})
	h.Describe(w, delegated)
}

func (h *ImageDescribeHandler) runtimeDescribeAgent(w http.ResponseWriter, r *http.Request, agentID uuid.UUID, bearer string) (*model.Agent, *model.Sandbox, bool) {
	var agent model.Agent
	if err := h.db.WithContext(r.Context()).Where("id = ? AND status <> ?", agentID, "archived").First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, imageDescribeError{Error: "agent not found", ErrorCode: "agent_not_found"})
			return nil, nil, false
		}
		writeJSON(w, http.StatusInternalServerError, imageDescribeError{Error: "failed to load agent", ErrorCode: "internal_error"})
		return nil, nil, false
	}
	if agent.OrgID == nil {
		writeJSON(w, http.StatusBadRequest, imageDescribeError{Error: "agent has no org", ErrorCode: "agent_missing_org"})
		return nil, nil, false
	}
	sandbox, err := (agentsandbox.Selector{DB: h.db}).MainRuntime(r.Context(), *agent.OrgID, agent.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, imageDescribeError{Error: "sandbox not found for agent", ErrorCode: "sandbox_not_found"})
			return nil, nil, false
		}
		writeJSON(w, http.StatusInternalServerError, imageDescribeError{Error: "failed to load sandbox", ErrorCode: "internal_error"})
		return nil, nil, false
	}
	want, err := h.runtimeSecrets.DecryptString(sandbox.EncryptedRuntimeSecret)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, imageDescribeError{Error: "failed to verify credentials", ErrorCode: "internal_error"})
		return nil, nil, false
	}
	if subtle.ConstantTimeCompare([]byte(bearer), []byte(want)) != 1 {
		writeJSON(w, http.StatusUnauthorized, imageDescribeError{Error: "invalid runtime secret", ErrorCode: "unauthorized"})
		return nil, nil, false
	}
	return &agent, sandbox, true
}

func (h *ImageDescribeHandler) runtimeDescribeOrg(w http.ResponseWriter, r *http.Request, orgID uuid.UUID) (*model.Org, bool) {
	var org model.Org
	if err := h.db.WithContext(r.Context()).Where("id = ?", orgID).First(&org).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, imageDescribeError{Error: "failed to load org", ErrorCode: "internal_error"})
		return nil, false
	}
	if !org.Active {
		writeJSON(w, http.StatusForbidden, imageDescribeError{Error: "organization is inactive", ErrorCode: "org_inactive"})
		return nil, false
	}
	return &org, true
}

func (h *ImageDescribeHandler) runtimeDescribeAssetAllowed(w http.ResponseWriter, r *http.Request, assetID, orgID, agentID, sandboxID uuid.UUID) bool {
	var asset model.AgentAsset
	if err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND agent_id = ? AND sandbox_id = ?", assetID, orgID, agentID, sandboxID).
		First(&asset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, imageDescribeError{Error: "asset not found", ErrorCode: "asset_not_found"})
			return false
		}
		writeJSON(w, http.StatusInternalServerError, imageDescribeError{Error: "failed to load asset", ErrorCode: "internal_error"})
		return false
	}
	return true
}
