package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/httpx"
	"github.com/usehivy/hivy/internal/microsandbox/model"
	"github.com/usehivy/hivy/internal/microsandbox/runner"
	"github.com/usehivy/hivy/internal/microsandbox/security"
)

type createTemplateRequest struct {
	OrgID        string            `json:"org_id"`
	Name         string            `json:"name"`
	BaseImageRef string            `json:"base_image_ref"`
	Size         string            `json:"size"`
	CPU          int               `json:"cpu"`
	MemoryMB     int               `json:"memory_mb"`
	DiskGB       int               `json:"disk_gb"`
	Commands     []string          `json:"commands"`
	Env          map[string]string `json:"env"`
}

func (s *Server) createTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createTemplateRequest
	if err := httpx.Decode(r, &req); err != nil || req.OrgID == "" || req.BaseImageRef == "" {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "org_id and base_image_ref are required"})
		return
	}
	id, err := newTemplateID()
	if err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to allocate template id"})
		return
	}
	buildID, err := security.ShortID("bld")
	if err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to allocate build id"})
		return
	}
	if req.Name == "" {
		req.Name = id
	}
	size := resolveTemplateSize(req)
	commands, _ := json.Marshal(req.Commands)

	var selected model.Runner
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		runner, err := selectRunnerForUpdate(tx, size)
		if err != nil {
			return err
		}
		selected = runner
		if err := reserveRunner(tx, &selected, size); err != nil {
			return err
		}
		return tx.Create(&model.Template{
			ID: id, OrgID: req.OrgID, RunnerID: selected.ID, Name: req.Name,
			BaseImageRef: req.BaseImageRef, Status: model.TemplateStatusBuilding,
			CommandsJSON: string(commands),
		}).Error
	})
	if err != nil {
		httpx.JSON(w, http.StatusServiceUnavailable, api.ErrorResponse{Error: err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	flush := func() {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	emit := func(event runner.TemplateBuildEvent) {
		_ = enc.Encode(event)
		flush()
	}
	emit(runner.TemplateBuildEvent{Type: "status", Status: "building", ID: id})

	var result runner.CreateTemplateResponse
	var templateErr error
	callErr := s.client.PostStream(ctx, selected.APIURL, "/v1/templates", runner.CreateTemplateRequest{
		ID: id, BuildID: "bld-" + buildID, OrgID: req.OrgID, Name: req.Name, BaseImageRef: req.BaseImageRef,
		Commands: req.Commands, Env: req.Env, CPU: size.CPU, MemoryMB: size.MemoryMB, DiskGB: size.DiskGB,
	}, func(raw []byte) error {
		var event runner.TemplateBuildEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			return err
		}
		switch event.Type {
		case "log":
			appendTemplateLog(ctx, s.db, id, event.Message)
		case "error":
			templateErr = errors.New(event.Message)
			_ = s.db.WithContext(ctx).Model(&model.Template{}).Where("id = ?", id).Updates(map[string]any{
				"status": model.TemplateStatusError, "error_message": event.Message,
			}).Error
		case "result":
			result.ID = event.ID
			result.ImageRef = event.ImageRef
			result.ImageDigest = event.ImageDigest
			result.ValidationSandboxID = event.ValidationSandboxID
		}
		emit(event)
		return nil
	})
	if callErr != nil {
		templateErr = callErr
	}
	if templateErr == nil && (strings.TrimSpace(result.ImageRef) == "" || strings.TrimSpace(result.ImageDigest) == "") {
		templateErr = errors.New("runner template build did not return image metadata")
	}

	_ = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_ = releaseRunner(tx, &selected, size)
		updates := map[string]any{}
		if templateErr != nil {
			updates["status"] = model.TemplateStatusError
			updates["error_message"] = templateErr.Error()
		} else {
			updates["status"] = model.TemplateStatusReady
			updates["image_ref"] = result.ImageRef
			updates["image_digest"] = result.ImageDigest
			updates["validation_sandbox_id"] = result.ValidationSandboxID
			updates["error_message"] = ""
		}
		return tx.Model(&model.Template{}).Where("id = ?", id).Updates(updates).Error
	})

	if templateErr != nil {
		emit(runner.TemplateBuildEvent{Type: "error", Status: "error", ID: id, Message: templateErr.Error()})
		return
	}
	emit(runner.TemplateBuildEvent{
		Type: "result", Status: "ready", ID: id, ImageRef: result.ImageRef, ImageDigest: result.ImageDigest,
		ValidationSandboxID: result.ValidationSandboxID,
	})
}

func (s *Server) getTemplate(w http.ResponseWriter, r *http.Request) {
	template, err := s.loadTemplateByID(r.Context(), chi.URLParam(r, "templateID"))
	if err != nil {
		httpx.JSON(w, http.StatusNotFound, api.ErrorResponse{Error: "template not found"})
		return
	}
	httpx.JSON(w, http.StatusOK, template)
}

func (s *Server) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "templateID")
	var tmpl model.Template
	if err := s.db.WithContext(r.Context()).First(&tmpl, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httpx.JSON(w, http.StatusNotFound, api.ErrorResponse{Error: "template not found"})
			return
		}
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to load template"})
		return
	}
	if err := s.db.WithContext(r.Context()).Delete(&tmpl).Error; err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to delete template"})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func newTemplateID() (string, error) {
	id, err := security.ShortID("tpl")
	if err != nil {
		return "", err
	}
	return "tpl_" + id, nil
}

func resolveTemplateSize(req createTemplateRequest) api.Size {
	sandboxReq := createSandboxRequest{Size: req.Size, CPU: req.CPU, MemoryMB: req.MemoryMB, DiskGB: req.DiskGB}
	return resolveSize(sandboxReq)
}

func appendTemplateLog(ctx context.Context, db *gorm.DB, id, line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	_ = db.WithContext(ctx).Model(&model.Template{}).Where("id = ?", id).
		Update("logs", gorm.Expr("logs || ?", line+"\n")).Error
}

func (s *Server) loadTemplateByID(ctx context.Context, id string) (model.Template, error) {
	var tmpl model.Template
	if !isTemplateID(id) {
		return tmpl, fmt.Errorf("not a template id")
	}
	if err := s.db.WithContext(ctx).First(&tmpl, "id = ?", id).Error; err != nil {
		return model.Template{}, err
	}
	return tmpl, nil
}

func isTemplateID(id string) bool {
	return strings.HasPrefix(strings.TrimSpace(id), "tpl_")
}
