package control

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/httpx"
	"github.com/usehivy/hivy/internal/microsandbox/model"
	"github.com/usehivy/hivy/internal/microsandbox/security"
)

type updateOrgPreviewPasswordRequest struct {
	PreviewPassword string `json:"preview_password"`
}

func (s *Server) getOrgPreviewPassword(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	var secret model.OrgPreviewSecret
	if err := s.db.First(&secret, "org_id = ?", orgID).Error; err != nil {
		httpx.JSON(w, http.StatusNotFound, api.ErrorResponse{Error: "preview password not found"})
		return
	}
	password, err := security.DecryptString(s.cfg.PreviewPasswordKey, secret.PasswordCiphertext)
	if err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to decrypt preview password"})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"org_id": orgID, "preview_password": password})
}

func (s *Server) updateOrgPreviewPassword(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	var req updateOrgPreviewPasswordRequest
	if err := httpx.Decode(r, &req); err != nil || req.PreviewPassword == "" {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "preview_password is required"})
		return
	}
	password, err := s.ensureOrgPassword(r.Context(), orgID, req.PreviewPassword)
	if err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to update preview password"})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"org_id": orgID, "preview_password": password})
}
