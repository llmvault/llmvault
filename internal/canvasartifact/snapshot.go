package canvasartifact

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func (s *Service) artifactResponse(ctx context.Context, artifact model.CanvasArtifact, includeFiles bool) (*ArtifactResponse, error) {
	resp := &ArtifactResponse{
		ID:        artifact.ID,
		Slug:      artifact.Slug,
		Type:      artifact.Type,
		Name:      artifact.Name,
		ProjectID: artifact.CanvasProjectID,
		Manifest:  artifact.ManifestJSON,
		UpdatedAt: artifact.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if !includeFiles {
		return resp, nil
	}
	files, err := s.fileResponses(ctx, artifact.ID, false)
	if err != nil {
		return nil, err
	}
	resp.Files = files
	return resp, nil
}

func (s *Service) fileResponses(ctx context.Context, artifactID uuid.UUID, includeDownloadURL bool) ([]ArtifactFileResponse, error) {
	var files []model.CanvasArtifactFile
	if err := s.db.WithContext(ctx).
		Where("canvas_artifact_id = ? AND archived_at IS NULL", artifactID).
		Order("path ASC").
		Find(&files).Error; err != nil {
		return nil, fmt.Errorf("list canvas artifact files: %w", err)
	}
	out := make([]ArtifactFileResponse, 0, len(files))
	for _, file := range files {
		resp := ArtifactFileResponse{
			Path:        file.Path,
			Role:        file.Role,
			ContentType: file.ContentType,
			ObjectKey:   file.ObjectKey,
			SizeBytes:   file.SizeBytes,
			SHA256:      file.SHA256,
		}
		if includeDownloadURL {
			if s.store == nil {
				return nil, ErrStorageNotConfigured
			}
			url, err := s.store.PresignGet(ctx, file.ObjectKey)
			if err != nil {
				return nil, fmt.Errorf("presign canvas artifact file: %w", err)
			}
			resp.DownloadURL = url
		}
		out = append(out, resp)
	}
	return out, nil
}

func (s *Service) snapshot(ctx context.Context, orgID uuid.UUID) (*SnapshotResponse, error) {
	var projects []model.CanvasProject
	if err := s.db.WithContext(ctx).
		Where("org_id = ? AND archived_at IS NULL AND slug <> ''", orgID).
		Order("updated_at DESC").
		Find(&projects).Error; err != nil {
		return nil, fmt.Errorf("list canvas snapshot projects: %w", err)
	}
	out := &SnapshotResponse{Projects: make([]SnapshotProject, 0, len(projects))}
	for _, project := range projects {
		snapProject := SnapshotProject{
			ID:        project.ID,
			Slug:      project.Slug,
			Name:      project.Name,
			Artifacts: []SnapshotArtifact{},
		}
		var artifacts []model.CanvasArtifact
		if err := s.db.WithContext(ctx).
			Where("org_id = ? AND canvas_project_id = ? AND archived_at IS NULL", orgID, project.ID).
			Order("updated_at DESC").
			Find(&artifacts).Error; err != nil {
			return nil, fmt.Errorf("list canvas snapshot artifacts: %w", err)
		}
		for _, artifact := range artifacts {
			files, err := s.fileResponses(ctx, artifact.ID, true)
			if err != nil {
				return nil, err
			}
			snapProject.Artifacts = append(snapProject.Artifacts, SnapshotArtifact{
				ID:       artifact.ID,
				Slug:     artifact.Slug,
				Type:     artifact.Type,
				Name:     artifact.Name,
				Manifest: artifact.ManifestJSON,
				Files:    files,
			})
		}
		out.Projects = append(out.Projects, snapProject)
	}
	return out, nil
}
