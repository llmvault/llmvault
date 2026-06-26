package canvasartifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func (s *Service) SyncArtifactForAgent(ctx context.Context, agentID uuid.UUID, req SyncRequest) (*SyncResponse, error) {
	if s.store == nil {
		return nil, ErrStorageNotConfigured
	}
	agent, org, err := s.loadAgentOrg(ctx, agentID)
	if err != nil {
		return nil, err
	}
	projectSlug := normalizeSlug(req.Project.Slug)
	if projectSlug == "" && strings.TrimSpace(req.Project.Ref) != "" {
		if _, err := uuid.Parse(strings.TrimSpace(req.Project.Ref)); err != nil {
			projectSlug = normalizeSlug(req.Project.Ref)
		}
	}
	if projectSlug == "" {
		projectSlug = normalizeSlug(req.Project.Name)
	}
	if projectSlug == "" {
		return nil, fmt.Errorf("project.slug is required")
	}
	artifactSlug := normalizeSlug(req.Artifact.Slug)
	if artifactSlug == "" {
		artifactSlug = normalizeSlug(req.Artifact.Name)
	}
	if artifactSlug == "" {
		return nil, fmt.Errorf("artifact.slug is required")
	}
	if !supportedArtifactType(req.Artifact.Type) {
		return nil, fmt.Errorf("unsupported artifact type %q", req.Artifact.Type)
	}
	if len(req.Files) == 0 && len(req.DeletedPaths) == 0 {
		return nil, fmt.Errorf("files are required")
	}

	var project model.CanvasProject
	var artifact model.CanvasArtifact
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var loadErr error
		project, loadErr = s.upsertProject(ctx, tx, org.ID, agent.ID, req.Project, projectSlug)
		if loadErr != nil {
			return loadErr
		}
		artifact, loadErr = s.upsertArtifact(ctx, tx, org.ID, agent.ID, project.ID, req, artifactSlug)
		if loadErr != nil {
			return loadErr
		}
		if loadErr = tx.Model(&model.CanvasArtifactFile{}).
			Where("canvas_artifact_id = ? AND archived_at IS NULL", artifact.ID).
			Update("archived_at", nowPtr()).Error; loadErr != nil {
			return fmt.Errorf("archive canvas artifact files: %w", loadErr)
		}
		if len(req.Files) == 0 {
			if loadErr = tx.Model(&artifact).Updates(map[string]any{"archived_at": nowPtr()}).Error; loadErr != nil {
				return fmt.Errorf("archive canvas artifact: %w", loadErr)
			}
			return nil
		}
		for _, file := range req.Files {
			row, loadErr := s.storeArtifactFile(ctx, org.ID, project.Slug, artifact.Slug, artifact.ID, file)
			if loadErr != nil {
				return loadErr
			}
			if loadErr = tx.Create(&row).Error; loadErr != nil {
				return fmt.Errorf("persist canvas artifact file: %w", loadErr)
			}
		}
		return tx.Model(&artifact).Update("updated_at", gorm.Expr("now()")).Error
	})
	if err != nil {
		return nil, err
	}
	artifactResp, err := s.artifactResponse(ctx, artifact, true)
	if err != nil {
		return nil, err
	}
	projectResp, err := s.projectResponse(ctx, project)
	if err != nil {
		return nil, err
	}
	return &SyncResponse{Project: *projectResp, Artifact: *artifactResp}, nil
}

func (s *Service) upsertProject(ctx context.Context, tx *gorm.DB, orgID, agentID uuid.UUID, input SyncProjectInput, slug string) (model.CanvasProject, error) {
	var project model.CanvasProject
	id, _ := uuid.Parse(strings.TrimSpace(input.ID))
	if id == uuid.Nil {
		id, _ = uuid.Parse(strings.TrimSpace(input.Ref))
	}
	q := tx.WithContext(ctx).Where("org_id = ? AND archived_at IS NULL", orgID)
	if id != uuid.Nil {
		q = q.Where("id = ?", id)
	} else {
		q = q.Where("slug = ?", slug)
	}
	err := q.First(&project).Error
	if err == nil {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			name = project.Name
		}
		if err := tx.Model(&project).Updates(map[string]any{"name": name, "slug": slug}).Error; err != nil {
			return model.CanvasProject{}, fmt.Errorf("update canvas project: %w", err)
		}
		project.Name = name
		project.Slug = slug
		return project, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.CanvasProject{}, fmt.Errorf("load canvas project: %w", err)
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = slug
	}
	project = model.CanvasProject{
		OrgID:            orgID,
		PenpotProjectID:  uuid.New(),
		Slug:             slug,
		Name:             name,
		CreatedByAgentID: &agentID,
	}
	if err := tx.Create(&project).Error; err != nil {
		return model.CanvasProject{}, fmt.Errorf("create canvas project: %w", err)
	}
	return project, nil
}

func (s *Service) upsertArtifact(ctx context.Context, tx *gorm.DB, orgID, agentID, projectID uuid.UUID, req SyncRequest, slug string) (model.CanvasArtifact, error) {
	var artifact model.CanvasArtifact
	id, _ := uuid.Parse(strings.TrimSpace(req.Artifact.ID))
	q := tx.WithContext(ctx).Where("org_id = ? AND canvas_project_id = ? AND archived_at IS NULL", orgID, projectID)
	if id != uuid.Nil {
		q = q.Where("id = ?", id)
	} else {
		q = q.Where("slug = ?", slug)
	}
	err := q.First(&artifact).Error
	name := strings.TrimSpace(req.Artifact.Name)
	if name == "" {
		name = slug
	}
	if err == nil {
		updates := map[string]any{
			"slug":              slug,
			"type":              req.Artifact.Type,
			"name":              name,
			"manifest_json":     req.Artifact.Manifest,
			"source_session_id": req.SessionID,
		}
		if err := tx.Model(&artifact).Updates(updates).Error; err != nil {
			return model.CanvasArtifact{}, fmt.Errorf("update canvas artifact: %w", err)
		}
		artifact.Slug = slug
		artifact.Type = req.Artifact.Type
		artifact.Name = name
		artifact.ManifestJSON = req.Artifact.Manifest
		artifact.SourceSessionID = req.SessionID
		return artifact, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.CanvasArtifact{}, fmt.Errorf("load canvas artifact: %w", err)
	}
	artifact = model.CanvasArtifact{
		OrgID:            orgID,
		CanvasProjectID:  projectID,
		Slug:             slug,
		Type:             req.Artifact.Type,
		Name:             name,
		ManifestJSON:     req.Artifact.Manifest,
		SourceSessionID:  req.SessionID,
		CreatedByAgentID: &agentID,
	}
	if err := tx.Create(&artifact).Error; err != nil {
		return model.CanvasArtifact{}, fmt.Errorf("create canvas artifact: %w", err)
	}
	return artifact, nil
}

func (s *Service) storeArtifactFile(ctx context.Context, orgID uuid.UUID, projectSlug, artifactSlug string, artifactID uuid.UUID, input SyncFileInput) (model.CanvasArtifactFile, error) {
	cleanPath, err := cleanArtifactPath(input.Path)
	if err != nil {
		return model.CanvasArtifactFile{}, err
	}
	content := []byte(input.Content)
	hash := strings.TrimSpace(input.SHA256)
	if hash == "" {
		sum := sha256.Sum256(content)
		hash = hex.EncodeToString(sum[:])
	}
	contentType := strings.TrimSpace(input.ContentType)
	if contentType == "" {
		contentType = "text/html; charset=utf-8"
	}
	key := fmt.Sprintf("pub/o/%s/canvas/projects/%s/artifacts/%s/%s/%s", orgID, projectSlug, artifactSlug, hash, cleanPath)
	stored, err := s.store.Stream(ctx, key, contentType, bytes.NewReader(content))
	if err != nil {
		return model.CanvasArtifactFile{}, fmt.Errorf("store canvas artifact file: %w", err)
	}
	size := int64(len(content))
	if input.SizeBytes > 0 {
		size = input.SizeBytes
	}
	if stored != nil && stored.Bytes > 0 {
		size = stored.Bytes
	}
	return model.CanvasArtifactFile{
		OrgID:            orgID,
		CanvasArtifactID: artifactID,
		Path:             cleanPath,
		Role:             strings.TrimSpace(input.Role),
		ContentType:      contentType,
		ObjectKey:        key,
		SizeBytes:        size,
		SHA256:           hash,
	}, nil
}

func supportedArtifactType(value string) bool {
	return value == "web_page" || value == "presentation"
}

func cleanArtifactPath(value string) (string, error) {
	cleaned := path.Clean(strings.TrimSpace(value))
	if cleaned == "." || cleaned == "" || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return "", fmt.Errorf("invalid artifact file path %q", value)
	}
	return cleaned, nil
}
