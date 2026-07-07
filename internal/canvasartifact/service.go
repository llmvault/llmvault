package canvasartifact

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type Service struct {
	db    *gorm.DB
	store FileStore
}

func NewService(db *gorm.DB, store FileStore) *Service {
	return &Service{db: db, store: store}
}

func (s *Service) CreateProjectForAgent(ctx context.Context, agentID uuid.UUID, req ProjectCreateRequest) (*ProjectResponse, error) {
	agent, org, err := s.loadAgentOrg(ctx, agentID)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "Untitled project"
	}
	baseSlug := normalizeSlug(req.Slug)
	if baseSlug == "" {
		baseSlug = normalizeSlug(name)
	}
	slug, err := s.availableProjectSlug(ctx, org.ID, baseSlug)
	if err != nil {
		return nil, err
	}
	project := model.CanvasProject{
		OrgID:            org.ID,
		Slug:             slug,
		Name:             name,
		CreatedByAgentID: &agent.ID,
	}
	if err := s.db.WithContext(ctx).Create(&project).Error; err != nil {
		return nil, fmt.Errorf("create canvas project: %w", err)
	}
	return s.projectResponse(ctx, project, nil, nil)
}

func (s *Service) ListProjectsForAgent(ctx context.Context, agentID uuid.UUID) (*ProjectListResponse, error) {
	_, org, err := s.loadAgentOrg(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return s.ListProjectsForOrg(ctx, org.ID, nil, nil)
}

// ListProjectsForOrg lists an org's canvas projects. When visibleSessions is
// non-nil (a non-manager member), each project's artifacts are restricted to
// those whose source session the member may view, and projects left with no
// visible artifacts are dropped so their metadata does not leak. Nil means
// org-wide (manager or API-key caller).
func (s *Service) ListProjectsForOrg(ctx context.Context, orgID uuid.UUID, sessionID *uuid.UUID, visibleSessions *gorm.DB) (*ProjectListResponse, error) {
	var projects []model.CanvasProject
	q := s.db.WithContext(ctx).Where("org_id = ? AND archived_at IS NULL AND slug <> ''", orgID)
	if err := q.Order("updated_at DESC").Find(&projects).Error; err != nil {
		return nil, fmt.Errorf("list canvas projects: %w", err)
	}
	out := &ProjectListResponse{Projects: make([]ProjectResponse, 0, len(projects))}
	for _, project := range projects {
		resp, err := s.projectResponse(ctx, project, sessionID, visibleSessions)
		if err != nil {
			return nil, err
		}
		// Drop projects with no member-visible artifacts.
		if visibleSessions != nil && resp.ArtifactCount == 0 {
			continue
		}
		out.Projects = append(out.Projects, *resp)
	}
	return out, nil
}

func (s *Service) ListArtifactsForAgent(ctx context.Context, agentID uuid.UUID, filter ArtifactFilter) (*ArtifactListResponse, error) {
	_, org, err := s.loadAgentOrg(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return s.ListArtifactsForOrg(ctx, org.ID, filter)
}

func (s *Service) ListArtifactsForOrg(ctx context.Context, orgID uuid.UUID, filter ArtifactFilter) (*ArtifactListResponse, error) {
	var artifacts []model.CanvasArtifact
	q := s.db.WithContext(ctx).
		Preload("CanvasProject").
		Where("canvas_artifacts.org_id = ? AND canvas_artifacts.archived_at IS NULL", orgID)
	if filter.ProjectID != nil {
		q = q.Where("canvas_project_id = ?", *filter.ProjectID)
	}
	if filter.ProjectSlug != "" {
		q = q.Joins("JOIN canvas_projects cp ON cp.id = canvas_artifacts.canvas_project_id").
			Where("cp.slug = ? AND cp.archived_at IS NULL", filter.ProjectSlug)
	}
	if filter.SessionID != nil {
		q = q.Where("source_session_id = ?", *filter.SessionID)
	}
	if filter.VisibleSessionIDs != nil {
		q = q.Where("source_session_id IN (?)", filter.VisibleSessionIDs)
	}
	if err := q.Order("canvas_artifacts.updated_at DESC").Find(&artifacts).Error; err != nil {
		return nil, fmt.Errorf("list canvas artifacts: %w", err)
	}
	out := &ArtifactListResponse{Artifacts: make([]ArtifactResponse, 0, len(artifacts))}
	for _, artifact := range artifacts {
		resp, err := s.artifactResponse(ctx, artifact, false)
		if err != nil {
			return nil, err
		}
		out.Artifacts = append(out.Artifacts, *resp)
	}
	return out, nil
}

// GetArtifactForOrg loads one artifact. When visibleSessions is non-nil (a
// non-manager member), the artifact must also belong to a session the member
// may view, else it returns gorm.ErrRecordNotFound (handler maps that to 404).
func (s *Service) GetArtifactForOrg(ctx context.Context, orgID, artifactID uuid.UUID, visibleSessions *gorm.DB) (*ArtifactResponse, error) {
	q := s.db.WithContext(ctx).
		Where("org_id = ? AND id = ? AND archived_at IS NULL", orgID, artifactID)
	if visibleSessions != nil {
		q = q.Where("source_session_id IN (?)", visibleSessions)
	}
	var artifact model.CanvasArtifact
	if err := q.First(&artifact).Error; err != nil {
		return nil, err
	}
	return s.artifactResponse(ctx, artifact, true)
}

func (s *Service) SnapshotForAgent(ctx context.Context, agentID uuid.UUID) (*SnapshotResponse, error) {
	_, org, err := s.loadAgentOrg(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return s.snapshot(ctx, org.ID)
}

func (s *Service) loadAgentOrg(ctx context.Context, agentID uuid.UUID) (model.Agent, model.Org, error) {
	var agent model.Agent
	if err := s.db.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error; err != nil {
		return model.Agent{}, model.Org{}, err
	}
	if agent.OrgID == nil {
		return model.Agent{}, model.Org{}, gorm.ErrRecordNotFound
	}
	var org model.Org
	if err := s.db.WithContext(ctx).Where("id = ?", *agent.OrgID).First(&org).Error; err != nil {
		return model.Agent{}, model.Org{}, err
	}
	return agent, org, nil
}

func (s *Service) projectResponse(ctx context.Context, project model.CanvasProject, sessionID *uuid.UUID, visibleSessions *gorm.DB) (*ProjectResponse, error) {
	artifacts, err := s.projectArtifactResponses(ctx, project.OrgID, project.ID, sessionID, visibleSessions)
	if err != nil {
		return nil, err
	}
	return &ProjectResponse{
		ID:            project.ID,
		ProjectID:     project.ID,
		Slug:          project.Slug,
		Name:          project.Name,
		Description:   project.Description,
		ArtifactCount: int64(len(artifacts)),
		Artifacts:     artifacts,
	}, nil
}

func (s *Service) projectArtifactResponses(ctx context.Context, orgID, projectID uuid.UUID, sessionID *uuid.UUID, visibleSessions *gorm.DB) ([]ArtifactResponse, error) {
	var artifacts []model.CanvasArtifact
	q := s.db.WithContext(ctx).
		Where("org_id = ? AND canvas_project_id = ? AND archived_at IS NULL", orgID, projectID)
	if sessionID != nil {
		q = q.Where("source_session_id = ?", *sessionID)
	}
	if visibleSessions != nil {
		q = q.Where("source_session_id IN (?)", visibleSessions)
	}
	if err := q.Order("updated_at DESC").Find(&artifacts).Error; err != nil {
		return nil, fmt.Errorf("list project canvas artifacts: %w", err)
	}
	out := make([]ArtifactResponse, 0, len(artifacts))
	for _, artifact := range artifacts {
		resp, err := s.artifactResponse(ctx, artifact, false)
		if err != nil {
			return nil, err
		}
		out = append(out, *resp)
	}
	return out, nil
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func normalizeSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = slugPattern.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

func (s *Service) availableProjectSlug(ctx context.Context, orgID uuid.UUID, base string) (string, error) {
	if base == "" {
		base = "project"
	}
	for i := 0; i < 1000; i++ {
		slug := base
		if i > 0 {
			slug = fmt.Sprintf("%s-%d", base, i)
		}
		var count int64
		err := s.db.WithContext(ctx).Model(&model.CanvasProject{}).
			Where("org_id = ? AND slug = ? AND archived_at IS NULL", orgID, slug).
			Count(&count).Error
		if err != nil {
			return "", fmt.Errorf("check project slug: %w", err)
		}
		if count == 0 {
			return slug, nil
		}
	}
	return "", fmt.Errorf("could not allocate canvas project slug")
}

func nowPtr() *time.Time {
	now := time.Now().UTC()
	return &now
}
