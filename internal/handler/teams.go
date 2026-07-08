package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

type teamMutationRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

type teamResponse struct {
	ID           string            `json:"id"`
	OrgID        string            `json:"org_id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	CreatedBy    *string           `json:"created_by,omitempty"`
	MemberCount  int64             `json:"member_count"`
	ChannelCount int64             `json:"channel_count"`
	Channels     []channelResponse `json:"channels"`
	ArchivedAt   *string           `json:"archived_at,omitempty"`
	CreatedAt    string            `json:"created_at"`
	UpdatedAt    string            `json:"updated_at"`
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
// @Description Returns the active teams visible to the caller. Org managers and
// @Description API keys see every team in the organization; a plain member sees
// @Description only the teams they are an active member of.
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
	orgWide, userID, err := actorSeesOrgWide(r.Context(), h.db, org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list teams"})
		return
	}
	q := h.db.WithContext(r.Context()).
		Where("org_id = ? AND archived_at IS NULL", org.ID)
	if !orgWide {
		// A plain member sees only the teams they actively belong to.
		q = q.Where("id IN (?)", visibleTeamSubquery(h.db, userID))
	}
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
	h.attachTeamChannels(r.Context(), org.ID, out, orgWide, userID)
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
	// A new team is provisioned to be self-sufficient in one transaction: the
	// team row, the creator as an owner team member, then the team's default
	// Hivy agent and #general channel.
	err := h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&team).Error; err != nil {
			return err
		}
		if userID != nil {
			teamMember := model.TeamMember{
				OrgID:  org.ID,
				TeamID: team.ID,
				UserID: *userID,
				Role:   "owner",
			}
			if err := tx.Create(&teamMember).Error; err != nil {
				return fmt.Errorf("create team member: %w", err)
			}
		}
		var createdBy uuid.UUID
		if userID != nil {
			createdBy = *userID
		}
		if _, _, err := provisionTeamDefaults(r.Context(), tx, org.ID, team.ID, createdBy); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
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
// @Description Returns one active team and its members. Visible to org managers,
// @Description API keys, and members of that team; other members receive 404.
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
