package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// ListEnvironmentVariables handles GET /v1/orgs/current/environment-variables.
// @Summary List organization environment variables
// @Description Lists custom environment variables stored on the Hivy employee. Values are not returned.
// @Tags orgs
// @Produce json
// @Success 200 {object} orgEnvironmentVariablesResponse
// @Failure 403 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/environment-variables [get]
func (h *OrgHandler) ListEnvironmentVariables(w http.ResponseWriter, r *http.Request) {
	_, envVars, ok := h.loadOrgEnvironmentVars(w, r)
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, orgEnvironmentVariablesResponse{Data: customOrgEnvResponses(envVars)})
}

// CreateEnvironmentVariable handles POST /v1/orgs/current/environment-variables.
// @Summary Create an organization environment variable
// @Description Stores a custom environment variable on the Hivy employee. It is pushed to runtime sandboxes as HIVY_ORG_<NAME>.
// @Tags orgs
// @Accept json
// @Produce json
// @Param body body createOrgEnvironmentVariableRequest true "Environment variable"
// @Success 201 {object} orgEnvironmentVariableResponse
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/environment-variables [post]
func (h *OrgHandler) CreateEnvironmentVariable(w http.ResponseWriter, r *http.Request) {
	var req createOrgEnvironmentVariableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	name, err := normalizeOrgEnvName(req.Name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Value == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "value is required"})
		return
	}

	employee, envVars, ok := h.loadOrgEnvironmentVars(w, r)
	if !ok {
		return
	}
	envKey := orgEnvKey(name)
	if _, exists := envVars[envKey]; exists {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "environment variable already exists"})
		return
	}
	envVars[envKey] = req.Value

	if !h.saveAndSyncOrgEnvironmentVars(w, r, employee, envVars) {
		return
	}
	writeJSON(w, http.StatusCreated, orgEnvResponse(name))
}

// UpdateEnvironmentVariable handles PATCH /v1/orgs/current/environment-variables/{name}.
// @Summary Update an organization environment variable
// @Description Renames and/or updates a custom environment variable stored on the Hivy employee.
// @Tags orgs
// @Accept json
// @Produce json
// @Param name path string true "Environment variable name"
// @Param body body updateOrgEnvironmentVariableRequest true "Fields to patch"
// @Success 200 {object} orgEnvironmentVariableResponse
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/environment-variables/{name} [patch]
func (h *OrgHandler) UpdateEnvironmentVariable(w http.ResponseWriter, r *http.Request) {
	currentName, err := normalizeOrgEnvName(chi.URLParam(r, "name"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	var req updateOrgEnvironmentVariableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == nil && req.Value == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}
	if req.Value != nil && *req.Value == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "value cannot be empty"})
		return
	}

	newName := currentName
	if req.Name != nil {
		newName, err = normalizeOrgEnvName(*req.Name)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}

	employee, envVars, ok := h.loadOrgEnvironmentVars(w, r)
	if !ok {
		return
	}
	currentKey := orgEnvKey(currentName)
	currentValue, exists := envVars[currentKey]
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "environment variable not found"})
		return
	}
	if req.Value != nil {
		currentValue = *req.Value
	}

	newKey := orgEnvKey(newName)
	if newKey != currentKey {
		if _, conflict := envVars[newKey]; conflict {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "environment variable already exists"})
			return
		}
		delete(envVars, currentKey)
	}
	envVars[newKey] = currentValue

	if !h.saveAndSyncOrgEnvironmentVars(w, r, employee, envVars) {
		return
	}
	writeJSON(w, http.StatusOK, orgEnvResponse(newName))
}

// DeleteEnvironmentVariable handles DELETE /v1/orgs/current/environment-variables/{name}.
// @Summary Delete an organization environment variable
// @Description Removes a custom environment variable from the Hivy employee.
// @Tags orgs
// @Produce json
// @Param name path string true "Environment variable name"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/environment-variables/{name} [delete]
func (h *OrgHandler) DeleteEnvironmentVariable(w http.ResponseWriter, r *http.Request) {
	name, err := normalizeOrgEnvName(chi.URLParam(r, "name"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	employee, envVars, ok := h.loadOrgEnvironmentVars(w, r)
	if !ok {
		return
	}
	envKey := orgEnvKey(name)
	if _, exists := envVars[envKey]; !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "environment variable not found"})
		return
	}
	delete(envVars, envKey)

	if !h.saveAndSyncOrgEnvironmentVars(w, r, employee, envVars) {
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "deleted"})
}

func (h *OrgHandler) loadOrgEnvironmentVars(w http.ResponseWriter, r *http.Request) (*model.Employee, map[string]string, bool) {
	if h.envEncKey == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "environment variable encryption is not configured"})
		return nil, nil, false
	}
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no organization context"})
		return nil, nil, false
	}
	employee, err := ensureHivyEmployee(r.Context(), h.db, org.ID)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to load Hivy employee for environment variables", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load Hivy employee"})
		return nil, nil, false
	}

	envVars := map[string]string{}
	if len(employee.EncryptedEnvVars) == 0 {
		return employee, envVars, true
	}
	decrypted, err := h.envEncKey.DecryptString(employee.EncryptedEnvVars)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to decrypt Hivy employee environment variables", "error", err, "employee_id", employee.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load environment variables"})
		return nil, nil, false
	}
	if err := json.Unmarshal([]byte(decrypted), &envVars); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to decode Hivy employee environment variables", "error", err, "employee_id", employee.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load environment variables"})
		return nil, nil, false
	}
	return employee, envVars, true
}

func (h *OrgHandler) saveAndSyncOrgEnvironmentVars(w http.ResponseWriter, r *http.Request, employee *model.Employee, envVars map[string]string) bool {
	encoded, err := json.Marshal(envVars)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encode environment variables"})
		return false
	}
	encrypted, err := h.envEncKey.EncryptString(string(encoded))
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to encrypt Hivy employee environment variables", "error", err, "employee_id", employee.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save environment variables"})
		return false
	}
	if err := h.db.WithContext(r.Context()).
		Model(&model.Employee{}).
		Where("id = ?", employee.ID).
		Update("encrypted_env_vars", encrypted).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to save Hivy employee environment variables", "error", err, "employee_id", employee.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save environment variables"})
		return false
	}

	if h.employeeSyncer == nil {
		return true
	}
	if err := h.employeeSyncer.SyncOrgHivyEmployee(r.Context(), *employee.OrgID); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to sync Hivy employee environment variables", "error", err, "employee_id", employee.ID, "org_id", employee.OrgID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "environment variable saved, but failed to sync employee sandbox"})
		return false
	}
	return true
}
