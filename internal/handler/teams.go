package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type TeamHandler struct {
	db *gorm.DB
}

func NewTeamHandler(db *gorm.DB) *TeamHandler {
	return &TeamHandler{db: db}
}

type teamMutationRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

type teamMemberRequest struct {
	Role *string `json:"role,omitempty"`
}

type teamResponse struct {
	ID           string  `json:"id"`
	OrgID        string  `json:"org_id"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	CreatedBy    *string `json:"created_by,omitempty"`
	MemberCount  int64   `json:"member_count"`
	ChannelCount int64   `json:"channel_count"`
	ArchivedAt   *string `json:"archived_at,omitempty"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

type teamMemberResponse struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

type teamMutationResponse struct {
	Team teamResponse `json:"team"`
}

type teamDetailResponse struct {
	Team    teamResponse         `json:"team"`
	Members []teamMemberResponse `json:"members"`
}

// @Summary List teams
// @Description Returns active teams for the current organization. Admin-only.
// @Tags teams
// @Produce json
// @Param limit query int false "Maximum results to return"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[teamResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams [get]
func (h *TeamHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := orgForTeamRequest(w, r)
	if !ok {
		return
	}
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	q := h.db.WithContext(r.Context()).
		Where("org_id = ? AND archived_at IS NULL", org.ID)
	q = applyPagination(q, cursor, limit)

	var teams []model.Team
	if err := q.Find(&teams).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list teams"})
		return
	}
	hasMore := len(teams) > limit
	if hasMore {
		teams = teams[:limit]
	}
	out := h.teamResponses(r.Context(), teams)
	resp := paginatedResponse[teamResponse]{Data: out, HasMore: hasMore}
	if hasMore {
		last := teams[len(teams)-1]
		next := encodeCursor(last.CreatedAt, last.ID)
		resp.NextCursor = &next
	}
	writeJSON(w, http.StatusOK, resp)
}

// @Summary Create a team
// @Description Creates a team in the current organization. Admin-only.
// @Tags teams
// @Accept json
// @Produce json
// @Param body body teamMutationRequest true "Team parameters"
// @Success 201 {object} teamMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams [post]
func (h *TeamHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := orgForTeamRequest(w, r)
	if !ok {
		return
	}
	var req teamMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	name := normalizeTeamName(cleanStringPtr(req.Name))
	if name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "name is required"})
		return
	}
	userID, _ := currentRequestUserID(r.Context())
	team := model.Team{
		OrgID:       org.ID,
		Name:        name,
		Description: cleanStringPtr(req.Description),
		CreatedBy:   userID,
	}
	if err := h.db.WithContext(r.Context()).Create(&team).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "team already exists"})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "create team", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create team"})
		return
	}
	writeJSON(w, http.StatusCreated, teamMutationResponse{Team: h.teamResponse(r.Context(), team)})
}

// @Summary Get a team
// @Description Returns one active team and its members. Admin-only.
// @Tags teams
// @Produce json
// @Param id path string true "Team ID"
// @Success 200 {object} teamDetailResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{id} [get]
func (h *TeamHandler) Get(w http.ResponseWriter, r *http.Request) {
	team, ok := h.loadTeamForRequest(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, teamDetailResponse{
		Team:    h.teamResponse(r.Context(), team),
		Members: h.teamMembers(r.Context(), team.ID),
	})
}

// @Summary Update a team
// @Description Updates an active team in the current organization. Admin-only.
// @Tags teams
// @Accept json
// @Produce json
// @Param id path string true "Team ID"
// @Param body body teamMutationRequest true "Team parameters"
// @Success 200 {object} teamMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{id} [patch]
func (h *TeamHandler) Update(w http.ResponseWriter, r *http.Request) {
	team, ok := h.loadTeamForRequest(w, r)
	if !ok {
		return
	}
	var req teamMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	updates := map[string]any{}
	if req.Name != nil {
		name := normalizeTeamName(*req.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "name cannot be empty"})
			return
		}
		updates["name"] = name
		team.Name = name
	}
	if req.Description != nil {
		value := cleanStringPtr(req.Description)
		updates["description"] = value
		team.Description = value
	}
	if len(updates) > 0 {
		if err := h.db.WithContext(r.Context()).
			Model(&model.Team{}).
			Where("id = ? AND org_id = ?", team.ID, team.OrgID).
			Updates(updates).Error; err != nil {
			if isDuplicateKeyError(err) {
				writeJSON(w, http.StatusConflict, errorResponse{Error: "team already exists"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update team"})
			return
		}
	}
	writeJSON(w, http.StatusOK, teamMutationResponse{Team: h.teamResponse(r.Context(), team)})
}

// @Summary Archive a team
// @Description Archives an active team after all channels are removed from it. Admin-only.
// @Tags teams
// @Produce json
// @Param id path string true "Team ID"
// @Success 200 {object} teamMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{id} [delete]
func (h *TeamHandler) Archive(w http.ResponseWriter, r *http.Request) {
	team, ok := h.loadTeamForRequest(w, r)
	if !ok {
		return
	}
	var channelCount int64
	if err := h.db.WithContext(r.Context()).
		Model(&model.Channel{}).
		Where("org_id = ? AND team_id = ? AND archived_at IS NULL", team.OrgID, team.ID).
		Count(&channelCount).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to check team channels"})
		return
	}
	if channelCount > 0 {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "remove channels from this team before archiving it"})
		return
	}
	now := time.Now()
	if err := h.db.WithContext(r.Context()).
		Model(&model.Team{}).
		Where("id = ? AND org_id = ?", team.ID, team.OrgID).
		Update("archived_at", &now).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to archive team"})
		return
	}
	team.ArchivedAt = &now
	writeJSON(w, http.StatusOK, teamMutationResponse{Team: h.teamResponse(r.Context(), team)})
}

// @Summary Add or update a team member
// @Description Adds an existing organization member to a team or updates their team role. Admin-only.
// @Tags teams
// @Accept json
// @Produce json
// @Param id path string true "Team ID"
// @Param userID path string true "User ID"
// @Param body body teamMemberRequest false "Team member parameters"
// @Success 200 {object} teamDetailResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{id}/members/{userID} [put]
func (h *TeamHandler) PutMember(w http.ResponseWriter, r *http.Request) {
	team, ok := h.loadTeamForRequest(w, r)
	if !ok {
		return
	}
	targetUserID, ok := teamUserIDFromRequest(w, r)
	if !ok {
		return
	}
	if !userBelongsToOrg(r.Context(), h.db, team.OrgID, targetUserID) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "user must belong to this org"})
		return
	}
	req := teamMemberRequest{}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	role := defaultString(cleanStringPtr(req.Role), "member")
	if !validTeamMemberRole(role) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "role must be owner or member"})
		return
	}
	member := model.TeamMember{OrgID: team.OrgID, TeamID: team.ID, UserID: targetUserID, Role: role}
	if err := h.db.WithContext(r.Context()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "team_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"role", "updated_at"}),
	}).Create(&member).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update team member"})
		return
	}
	writeJSON(w, http.StatusOK, teamDetailResponse{
		Team:    h.teamResponse(r.Context(), team),
		Members: h.teamMembers(r.Context(), team.ID),
	})
}

// @Summary Remove a team member
// @Description Removes an organization member from a team. Admin-only.
// @Tags teams
// @Produce json
// @Param id path string true "Team ID"
// @Param userID path string true "User ID"
// @Success 200 {object} teamDetailResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{id}/members/{userID} [delete]
func (h *TeamHandler) DeleteMember(w http.ResponseWriter, r *http.Request) {
	team, ok := h.loadTeamForRequest(w, r)
	if !ok {
		return
	}
	targetUserID, ok := teamUserIDFromRequest(w, r)
	if !ok {
		return
	}
	if err := h.db.WithContext(r.Context()).
		Where("team_id = ? AND user_id = ?", team.ID, targetUserID).
		Delete(&model.TeamMember{}).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to remove team member"})
		return
	}
	writeJSON(w, http.StatusOK, teamDetailResponse{
		Team:    h.teamResponse(r.Context(), team),
		Members: h.teamMembers(r.Context(), team.ID),
	})
}

func (h *TeamHandler) loadTeamForRequest(w http.ResponseWriter, r *http.Request) (model.Team, bool) {
	org, ok := orgForTeamRequest(w, r)
	if !ok {
		return model.Team{}, false
	}
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return model.Team{}, false
	}
	var team model.Team
	err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", teamID, org.ID).
		First(&team).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "team not found"})
		return model.Team{}, false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load team"})
		return model.Team{}, false
	}
	return team, true
}

func orgForTeamRequest(w http.ResponseWriter, r *http.Request) (*model.Org, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return nil, false
	}
	return org, true
}

func teamIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	teamID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid team id"})
		return uuid.Nil, false
	}
	return teamID, true
}

func teamUserIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid user id"})
		return uuid.Nil, false
	}
	return userID, true
}

func normalizeTeamName(raw string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
}

func validTeamMemberRole(role string) bool {
	return role == "owner" || role == "member"
}

func (h *TeamHandler) teamResponses(ctx context.Context, teams []model.Team) []teamResponse {
	counts := h.teamCounts(ctx, teamIDs(teams))
	out := make([]teamResponse, len(teams))
	for i, team := range teams {
		out[i] = teamToResponse(team, counts.members[team.ID], counts.channels[team.ID])
	}
	return out
}

func (h *TeamHandler) teamResponse(ctx context.Context, team model.Team) teamResponse {
	counts := h.teamCounts(ctx, []uuid.UUID{team.ID})
	return teamToResponse(team, counts.members[team.ID], counts.channels[team.ID])
}

type teamCountMaps struct {
	members  map[uuid.UUID]int64
	channels map[uuid.UUID]int64
}

func (h *TeamHandler) teamCounts(ctx context.Context, ids []uuid.UUID) teamCountMaps {
	out := teamCountMaps{
		members:  make(map[uuid.UUID]int64, len(ids)),
		channels: make(map[uuid.UUID]int64, len(ids)),
	}
	if len(ids) == 0 {
		return out
	}
	type countRow struct {
		TeamID uuid.UUID
		Count  int64
	}
	var memberRows []countRow
	_ = h.db.WithContext(ctx).
		Model(&model.TeamMember{}).
		Select("team_id, count(*) AS count").
		Where("team_id IN ?", ids).
		Group("team_id").
		Scan(&memberRows).Error
	for _, row := range memberRows {
		out.members[row.TeamID] = row.Count
	}
	var channelRows []countRow
	_ = h.db.WithContext(ctx).
		Model(&model.Channel{}).
		Select("team_id, count(*) AS count").
		Where("team_id IN ? AND archived_at IS NULL", ids).
		Group("team_id").
		Scan(&channelRows).Error
	for _, row := range channelRows {
		out.channels[row.TeamID] = row.Count
	}
	return out
}

func (h *TeamHandler) teamMembers(ctx context.Context, teamID uuid.UUID) []teamMemberResponse {
	var members []model.TeamMember
	_ = h.db.WithContext(ctx).
		Preload("User").
		Where("team_id = ?", teamID).
		Order("created_at ASC").
		Find(&members).Error
	out := make([]teamMemberResponse, len(members))
	for i, member := range members {
		out[i] = teamMemberResponse{
			UserID:    member.UserID.String(),
			Email:     member.User.Email,
			Name:      member.User.Name,
			Role:      member.Role,
			CreatedAt: member.CreatedAt.Format(time.RFC3339),
		}
	}
	return out
}

func teamIDs(teams []model.Team) []uuid.UUID {
	ids := make([]uuid.UUID, len(teams))
	for i, team := range teams {
		ids[i] = team.ID
	}
	return ids
}

func teamToResponse(team model.Team, memberCount, channelCount int64) teamResponse {
	return teamResponse{
		ID:           team.ID.String(),
		OrgID:        team.OrgID.String(),
		Name:         team.Name,
		Description:  team.Description,
		CreatedBy:    formatUUIDPtr(team.CreatedBy),
		MemberCount:  memberCount,
		ChannelCount: channelCount,
		ArchivedAt:   formatTimePtr(team.ArchivedAt),
		CreatedAt:    team.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    team.UpdatedAt.Format(time.RFC3339),
	}
}

func userBelongsToOrg(ctx context.Context, db *gorm.DB, orgID, userID uuid.UUID) bool {
	var count int64
	_ = db.WithContext(ctx).
		Model(&model.OrgMembership{}).
		Where("org_id = ? AND user_id = ?", orgID, userID).
		Count(&count).Error
	return count == 1
}
