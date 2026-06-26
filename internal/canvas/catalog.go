package canvas

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type ProjectCatalogResult struct {
	Projects []ProjectCatalogProject `json:"projects"`
}

type ProjectCatalogProject struct {
	ProjectID uuid.UUID            `json:"project_id"`
	Name      string               `json:"name"`
	Files     []ProjectCatalogFile `json:"files"`
}

type ProjectCatalogFile struct {
	FileID       uuid.UUID  `json:"file_id"`
	ProjectID    uuid.UUID  `json:"project_id"`
	PageID       *uuid.UUID `json:"page_id,omitempty"`
	Name         string     `json:"name"`
	WorkspaceURL string     `json:"workspace_url"`
}

func (s *Service) ListProjectCatalogForOrg(ctx context.Context, orgID uuid.UUID) (*ProjectCatalogResult, error) {
	if !s.Enabled() {
		return nil, ErrNotConfigured
	}

	var projects []model.CanvasProject
	if err := s.db.WithContext(ctx).
		Where("org_id = ?", orgID).
		Order("updated_at DESC").
		Find(&projects).Error; err != nil {
		return nil, fmt.Errorf("list canvas projects: %w", err)
	}

	var files []model.CanvasFile
	if err := s.db.WithContext(ctx).
		Where("org_id = ?", orgID).
		Order("updated_at DESC").
		Find(&files).Error; err != nil {
		return nil, fmt.Errorf("list canvas files: %w", err)
	}

	teamID := TeamIDForOrg(orgID)
	filesByProject := make(map[uuid.UUID][]ProjectCatalogFile, len(projects))
	for _, file := range files {
		filesByProject[file.PenpotProjectID] = append(filesByProject[file.PenpotProjectID], ProjectCatalogFile{
			FileID:       file.PenpotFileID,
			ProjectID:    file.PenpotProjectID,
			PageID:       file.PenpotPageID,
			Name:         file.Name,
			WorkspaceURL: s.client.WorkspaceURL(teamID, file.PenpotFileID, file.PenpotPageID),
		})
	}

	result := &ProjectCatalogResult{Projects: make([]ProjectCatalogProject, 0, len(projects))}
	for _, project := range projects {
		projectFiles := filesByProject[project.PenpotProjectID]
		if projectFiles == nil {
			projectFiles = []ProjectCatalogFile{}
		}
		result.Projects = append(result.Projects, ProjectCatalogProject{
			ProjectID: project.PenpotProjectID,
			Name:      project.Name,
			Files:     projectFiles,
		})
	}
	return result, nil
}
