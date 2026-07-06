package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type ChannelHandler struct {
	db                  *gorm.DB
	externalProvisioner ChannelExternalProvisioner
	envEncKey           *crypto.SymmetricKey
	enqueuer            enqueue.TaskEnqueuer
}

func NewChannelHandler(db *gorm.DB, opts ...ChannelHandlerOption) *ChannelHandler {
	h := &ChannelHandler{db: db}
	for _, opt := range opts {
		if opt != nil {
			opt(h)
		}
	}
	return h
}

type channelMutationRequest struct {
	Name                 *string     `json:"name,omitempty"`
	Description          *string     `json:"description,omitempty"`
	Category             *string     `json:"category,omitempty"`
	MemoryMission        *string     `json:"memory_mission,omitempty"`
	Visibility           *string     `json:"visibility,omitempty"`
	TeamID               *string     `json:"team_id,omitempty"`
	DefaultAgentID       *string     `json:"default_agent_id,omitempty"`
	ImageModel           *string     `json:"image_model,omitempty"`
	VectorImageModel     *string     `json:"vector_image_model,omitempty"`
	Origin               *string     `json:"origin,omitempty"`
	ExternalProvider     *string     `json:"external_provider,omitempty"`
	ExternalConnectionID *string     `json:"external_connection_id,omitempty"`
	ExternalWorkspaceKey *string     `json:"external_workspace_key,omitempty"`
	ExternalResourceType *string     `json:"external_resource_type,omitempty"`
	ExternalResourceKey  *string     `json:"external_resource_key,omitempty"`
	ExternalResourceName *string     `json:"external_resource_name,omitempty"`
	ExternalResourceURL  *string     `json:"external_resource_url,omitempty"`
	ExternalMetadata     *model.JSON `json:"external_metadata,omitempty"`
}

type channelMutationResponse struct {
	Channel channelResponse `json:"channel"`
}

type channelDetailResponse struct {
	Channel channelResponse         `json:"channel"`
	Members []channelMemberResponse `json:"members"`
}

type channelResponse struct {
	ID                       string             `json:"id"`
	Name                     string             `json:"name"`
	Description              string             `json:"description"`
	Category                 string             `json:"category"`
	MemoryMission            string             `json:"memory_mission"`
	Kind                     string             `json:"kind"`
	Visibility               string             `json:"visibility"`
	TeamID                   *string            `json:"team_id,omitempty"`
	DefaultAgentID           string             `json:"default_agent_id"`
	ImageModel               string             `json:"image_model"`
	VectorImageModel         string             `json:"vector_image_model"`
	IsDefault                bool               `json:"is_default"`
	Origin                   string             `json:"origin"`
	ExternalProvider         string             `json:"external_provider"`
	ExternalConnectionID     *string            `json:"external_connection_id,omitempty"`
	ExternalWorkspaceKey     string             `json:"external_workspace_key"`
	ExternalResourceType     string             `json:"external_resource_type"`
	ExternalResourceKey      string             `json:"external_resource_key"`
	ExternalResourceName     string             `json:"external_resource_name"`
	ExternalResourceURL      string             `json:"external_resource_url"`
	ExternalMetadata         model.JSON         `json:"external_metadata"`
	CreatedBy                *string            `json:"created_by,omitempty"`
	Role                     string             `json:"role,omitempty"`
	MemberCount              int64              `json:"member_count"`
	RecentSessions           *[]sessionResponse `json:"recent_sessions,omitempty"`
	RecentSessionsHasMore    bool               `json:"recent_sessions_has_more,omitempty"`
	RecentSessionsNextCursor *string            `json:"recent_sessions_next_cursor,omitempty"`
	ArchivedAt               *string            `json:"archived_at,omitempty"`
	CreatedAt                string             `json:"created_at"`
	UpdatedAt                string             `json:"updated_at"`
}

type channelMemberResponse struct {
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

type channelMemberRequest struct {
	Role *string `json:"role,omitempty"`
}

func channelIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	channelID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid channel id"})
		return uuid.Nil, false
	}
	return channelID, true
}

func channelUserIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid user id"})
		return uuid.Nil, false
	}
	return userID, true
}

func currentRequestUserID(ctx context.Context) (*uuid.UUID, bool) {
	raw := middleware.UserID(ctx)
	if strings.TrimSpace(raw) == "" {
		return nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, false
	}
	return &id, true
}

func isAPIKeyRequest(ctx context.Context) bool {
	_, ok := middleware.APIKeyClaimsFromContext(ctx)
	return ok
}

func normalizeChannelName(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.TrimLeft(value, "#")
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.ContainsAny(value, " \t\n\r") {
		value = strings.Join(strings.Fields(value), "-")
	}
	return value
}

const systemChannelName = "system"

func isReservedChannelName(name string) bool {
	return name == systemChannelName
}

func formatUUIDPtr(id *uuid.UUID) *string {
	if id == nil || *id == uuid.Nil {
		return nil
	}
	value := id.String()
	return &value
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	value := t.Format(time.RFC3339)
	return &value
}

func channelToResponse(channel model.Channel, role string, memberCount int64) channelResponse {
	memoryMission := ""
	if channel.MemoryMission != nil {
		memoryMission = *channel.MemoryMission
	}
	return channelResponse{
		ID:                   channel.ID.String(),
		Name:                 channel.Name,
		Description:          channel.Description,
		Category:             channel.Category,
		MemoryMission:        memoryMission,
		Kind:                 channel.Kind,
		Visibility:           channel.Visibility,
		TeamID:               formatUUIDPtr(channel.TeamID),
		DefaultAgentID:       channel.DefaultAgentID.String(),
		ImageModel:           channel.ImageModel,
		VectorImageModel:     channel.VectorImageModel,
		IsDefault:            channel.IsDefault,
		Origin:               channel.Origin,
		ExternalProvider:     channel.ExternalProvider,
		ExternalConnectionID: formatUUIDPtr(channel.ExternalConnectionID),
		ExternalWorkspaceKey: channel.ExternalWorkspaceKey,
		ExternalResourceType: channel.ExternalResourceType,
		ExternalResourceKey:  channel.ExternalResourceKey,
		ExternalResourceName: channel.ExternalResourceName,
		ExternalResourceURL:  channel.ExternalResourceURL,
		ExternalMetadata:     channel.ExternalMetadata,
		CreatedBy:            formatUUIDPtr(channel.CreatedBy),
		Role:                 role,
		MemberCount:          memberCount,
		ArchivedAt:           formatTimePtr(channel.ArchivedAt),
		CreatedAt:            channel.CreatedAt.Format(time.RFC3339),
		UpdatedAt:            channel.UpdatedAt.Format(time.RFC3339),
	}
}
