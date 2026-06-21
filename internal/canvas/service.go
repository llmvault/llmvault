package canvas

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
)

var profileNamespace = uuid.MustParse("4f6c136f-5718-48e4-9f84-6d2d847d45cc")

var ErrNotConfigured = errors.New("canvas is not configured")

type Service struct {
	db     *gorm.DB
	client *Client
}

type RuntimeEnv map[string]string

type SessionURLResult struct {
	URL          string     `json:"url"`
	ExpiresIn    int64      `json:"expires_in"`
	CanvasFileID uuid.UUID  `json:"canvas_file_id"`
	PenpotFileID uuid.UUID  `json:"penpot_file_id"`
	PageID       *uuid.UUID `json:"page_id,omitempty"`
	TeamID       uuid.UUID  `json:"team_id"`
}

type ProjectCreateResult struct {
	ProjectID       uuid.UUID `json:"project_id"`
	PenpotProjectID uuid.UUID `json:"penpot_project_id"`
	TeamID          uuid.UUID `json:"team_id"`
	Name            string    `json:"name"`
	WorkspaceURL    string    `json:"workspace_url,omitempty"`
}

type FileCreateResult struct {
	FileID          uuid.UUID  `json:"file_id"`
	ProjectID       uuid.UUID  `json:"project_id"`
	PenpotFileID    uuid.UUID  `json:"penpot_file_id"`
	PenpotProjectID uuid.UUID  `json:"penpot_project_id"`
	PageID          *uuid.UUID `json:"page_id,omitempty"`
	TeamID          uuid.UUID  `json:"team_id"`
	Name            string     `json:"name"`
	WorkspaceURL    string     `json:"workspace_url"`
	SessionURL      string     `json:"session_url"`
}

func NewService(db *gorm.DB, client *Client) *Service {
	return &Service{db: db, client: client}
}

func (s *Service) Enabled() bool {
	return s != nil && s.db != nil && s.client != nil && s.client.Enabled()
}

func TeamIDForOrg(orgID uuid.UUID) uuid.UUID {
	return orgID
}

func HumanProfileID(userID, orgID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(profileNamespace, []byte("user:"+userID.String()+":"+orgID.String()))
}

func AgentProfileID(agentID, orgID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(profileNamespace, []byte("agent:"+agentID.String()+":"+orgID.String()))
}

func HivyID(id, orgID uuid.UUID) string {
	return id.String() + "-" + orgID.String()
}

func (s *Service) SyncCanvasOrg(ctx context.Context, orgID uuid.UUID) error {
	if !s.Enabled() {
		return nil
	}
	var org model.Org
	if err := s.db.WithContext(ctx).Where("id = ?", orgID).First(&org).Error; err != nil {
		return fmt.Errorf("load org: %w", err)
	}
	if _, err := s.upsertTeam(ctx, org); err != nil {
		return err
	}
	var memberships []model.OrgMembership
	if err := s.db.WithContext(ctx).Preload("User").Where("org_id = ?", orgID).Find(&memberships).Error; err != nil {
		return fmt.Errorf("load org members: %w", err)
	}
	for _, membership := range memberships {
		if membership.User.ID == uuid.Nil {
			continue
		}
		if _, err := s.upsertUserProfile(ctx, org.ID, membership.User); err != nil {
			return err
		}
	}
	var agents []model.Agent
	if err := s.db.WithContext(ctx).
		Where("org_id = ? AND status <> ?", orgID, "archived").
		Find(&agents).Error; err != nil {
		return fmt.Errorf("load org agents: %w", err)
	}
	for _, agent := range agents {
		if _, err := s.upsertAgentProfile(ctx, org.ID, agent); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) AgentRuntimeEnv(ctx context.Context, agent *model.Agent) (map[string]string, error) {
	env := map[string]string{}
	if !s.Enabled() || agent == nil || agent.ID == uuid.Nil || agent.OrgID == nil {
		return env, nil
	}
	var org model.Org
	if err := s.db.WithContext(ctx).Where("id = ?", *agent.OrgID).First(&org).Error; err != nil {
		return nil, fmt.Errorf("load canvas org: %w", err)
	}
	if _, err := s.upsertTeam(ctx, org); err != nil {
		return nil, err
	}
	profile, err := s.upsertAgentProfile(ctx, org.ID, *agent)
	if err != nil {
		return nil, err
	}
	token, err := s.client.MintSessionJWT(profile.ProfileID, TeamIDForOrg(org.ID), nil, nil)
	if err != nil {
		return nil, err
	}
	env[agentruntime.AgentEnvPenpotCanvasURL] = s.client.PublicURL
	env[agentruntime.AgentEnvPenpotCanvasTeamID] = TeamIDForOrg(org.ID).String()
	env[agentruntime.AgentEnvPenpotCanvasProfileID] = profile.ProfileID.String()
	env[agentruntime.AgentEnvPenpotCanvasSessionJWT] = token
	env[agentruntime.AgentEnvPenpotCanvasMCPURL] = profile.MCPURL
	return env, nil
}

func (s *Service) SessionURLForUser(ctx context.Context, orgID, userID, canvasFileID uuid.UUID, pageID *uuid.UUID) (*SessionURLResult, error) {
	if !s.Enabled() {
		return nil, ErrNotConfigured
	}
	var file model.CanvasFile
	if err := s.db.WithContext(ctx).Where("id = ? AND org_id = ?", canvasFileID, orgID).First(&file).Error; err != nil {
		return nil, err
	}
	var user model.User
	if err := s.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, fmt.Errorf("load user: %w", err)
	}
	var org model.Org
	if err := s.db.WithContext(ctx).Where("id = ?", orgID).First(&org).Error; err != nil {
		return nil, fmt.Errorf("load org: %w", err)
	}
	if _, err := s.upsertTeam(ctx, org); err != nil {
		return nil, err
	}
	profile, err := s.upsertUserProfile(ctx, orgID, user)
	if err != nil {
		return nil, err
	}
	targetPageID := pageID
	if targetPageID == nil {
		targetPageID = file.PenpotPageID
	}
	token, err := s.client.MintSessionJWT(profile.ProfileID, TeamIDForOrg(orgID), &file.PenpotFileID, targetPageID)
	if err != nil {
		return nil, err
	}
	return &SessionURLResult{
		URL:          s.client.SessionURL(token),
		ExpiresIn:    int64(SessionTTL.Seconds()),
		CanvasFileID: file.ID,
		PenpotFileID: file.PenpotFileID,
		PageID:       targetPageID,
		TeamID:       TeamIDForOrg(orgID),
	}, nil
}

func (s *Service) CreateProjectForAgent(ctx context.Context, agentID uuid.UUID, name string) (*ProjectCreateResult, error) {
	if !s.Enabled() {
		return nil, ErrNotConfigured
	}
	agent, org, _, err := s.ensureAgentProfile(ctx, agentID)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Untitled project"
	}
	penpotProjectID := uuid.New()
	created, err := s.client.UpsertProject(ctx, ProjectInput{
		ProjectID: penpotProjectID,
		TeamID:    TeamIDForOrg(org.ID),
		Name:      name,
	})
	if err != nil {
		return nil, err
	}
	if created.ProjectID != uuid.Nil {
		penpotProjectID = created.ProjectID
	}
	project := model.CanvasProject{
		OrgID:            org.ID,
		PenpotProjectID:  penpotProjectID,
		Name:             name,
		CreatedByAgentID: &agent.ID,
	}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "penpot_project_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "updated_at"}),
	}).Create(&project).Error; err != nil {
		return nil, fmt.Errorf("persist canvas project: %w", err)
	}
	var persisted model.CanvasProject
	if err := s.db.WithContext(ctx).Where("penpot_project_id = ?", penpotProjectID).First(&persisted).Error; err != nil {
		return nil, fmt.Errorf("load canvas project: %w", err)
	}
	return &ProjectCreateResult{
		ProjectID:       persisted.ID,
		PenpotProjectID: persisted.PenpotProjectID,
		TeamID:          TeamIDForOrg(org.ID),
		Name:            persisted.Name,
	}, nil
}

func (s *Service) CreateFileForAgent(ctx context.Context, agentID, projectID uuid.UUID, name string) (*FileCreateResult, error) {
	if !s.Enabled() {
		return nil, ErrNotConfigured
	}
	agent, org, profile, err := s.ensureAgentProfile(ctx, agentID)
	if err != nil {
		return nil, err
	}
	var project model.CanvasProject
	if err := s.db.WithContext(ctx).
		Where("org_id = ? AND (id = ? OR penpot_project_id = ?)", org.ID, projectID, projectID).
		First(&project).Error; err != nil {
		return nil, fmt.Errorf("load canvas project: %w", err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Untitled file"
	}
	penpotFileID := uuid.New()
	created, err := s.client.UpsertFile(ctx, FileInput{
		FileID:    penpotFileID,
		ProjectID: project.PenpotProjectID,
		ProfileID: &profile.ProfileID,
		Name:      name,
	})
	if err != nil {
		return nil, err
	}
	if created.FileID != uuid.Nil {
		penpotFileID = created.FileID
	}
	file := model.CanvasFile{
		OrgID:            org.ID,
		CanvasProjectID:  &project.ID,
		PenpotProjectID:  project.PenpotProjectID,
		PenpotFileID:     penpotFileID,
		Name:             name,
		CreatedByAgentID: &agent.ID,
	}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "penpot_file_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "canvas_project_id", "penpot_project_id", "updated_at"}),
	}).Create(&file).Error; err != nil {
		return nil, fmt.Errorf("persist canvas file: %w", err)
	}
	var persisted model.CanvasFile
	if err := s.db.WithContext(ctx).Where("penpot_file_id = ?", penpotFileID).First(&persisted).Error; err != nil {
		return nil, fmt.Errorf("load canvas file: %w", err)
	}
	token, err := s.client.MintSessionJWT(profile.ProfileID, TeamIDForOrg(org.ID), &persisted.PenpotFileID, persisted.PenpotPageID)
	if err != nil {
		return nil, err
	}
	return &FileCreateResult{
		FileID:          persisted.ID,
		ProjectID:       project.ID,
		PenpotFileID:    persisted.PenpotFileID,
		PenpotProjectID: project.PenpotProjectID,
		PageID:          persisted.PenpotPageID,
		TeamID:          TeamIDForOrg(org.ID),
		Name:            persisted.Name,
		WorkspaceURL:    s.client.WorkspaceURL(TeamIDForOrg(org.ID), persisted.PenpotFileID, persisted.PenpotPageID),
		SessionURL:      s.client.SessionURL(token),
	}, nil
}

func (s *Service) ensureAgentProfile(ctx context.Context, agentID uuid.UUID) (model.Agent, model.Org, *ProfileResult, error) {
	var agent model.Agent
	if err := s.db.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error; err != nil {
		return model.Agent{}, model.Org{}, nil, err
	}
	if agent.OrgID == nil {
		return model.Agent{}, model.Org{}, nil, gorm.ErrRecordNotFound
	}
	var org model.Org
	if err := s.db.WithContext(ctx).Where("id = ?", *agent.OrgID).First(&org).Error; err != nil {
		return model.Agent{}, model.Org{}, nil, err
	}
	if _, err := s.upsertTeam(ctx, org); err != nil {
		return model.Agent{}, model.Org{}, nil, err
	}
	profile, err := s.upsertAgentProfile(ctx, org.ID, agent)
	if err != nil {
		return model.Agent{}, model.Org{}, nil, err
	}
	return agent, org, profile, nil
}

func (s *Service) upsertTeam(ctx context.Context, org model.Org) (*TeamResult, error) {
	name := strings.TrimSpace(org.Name)
	if name == "" {
		name = "Hivy organization"
	}
	team, err := s.client.UpsertTeam(ctx, TeamInput{
		TeamID: TeamIDForOrg(org.ID),
		HivyID: org.ID.String(),
		Name:   name,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert canvas team: %w", err)
	}
	return team, nil
}

func (s *Service) upsertUserProfile(ctx context.Context, orgID uuid.UUID, user model.User) (*ProfileResult, error) {
	fullname := strings.TrimSpace(user.Name)
	if fullname == "" {
		fullname = strings.TrimSpace(user.Email)
	}
	profile, err := s.client.UpsertProfile(ctx, ProfileInput{
		ProfileID: HumanProfileID(user.ID, orgID),
		TeamID:    TeamIDForOrg(orgID),
		HivyID:    HivyID(user.ID, orgID),
		Email:     strings.TrimSpace(strings.ToLower(user.Email)),
		Fullname:  fullname,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert canvas user profile: %w", err)
	}
	return profile, nil
}

func (s *Service) upsertAgentProfile(ctx context.Context, orgID uuid.UUID, agent model.Agent) (*ProfileResult, error) {
	name := strings.TrimSpace(agent.Name)
	if name == "" {
		name = "Hivy agent"
	}
	email := fmt.Sprintf("agent-%s@canvas.usehivy.com", agent.ID.String())
	profile, err := s.client.UpsertProfile(ctx, ProfileInput{
		ProfileID: AgentProfileID(agent.ID, orgID),
		TeamID:    TeamIDForOrg(orgID),
		HivyID:    HivyID(agent.ID, orgID),
		Email:     email,
		Fullname:  name,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert canvas agent profile: %w", err)
	}
	return profile, nil
}

func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
