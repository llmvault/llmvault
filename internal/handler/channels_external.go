package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
)

type ChannelHandlerOption func(*ChannelHandler)

type ChannelExternalProvisioner interface {
	EnsureExternalChannel(context.Context, uuid.UUID, ChannelExternalProvisionRequest) error
}

type ChannelExternalProvisionRequest struct {
	Provider     string
	ConnectionID uuid.UUID
	WorkspaceKey string
	ResourceType string
	ResourceKey  string
	ResourceName string
}

type ChannelExternalProvisionError struct {
	StatusCode int
	Message    string
	Err        error
}

func (e *ChannelExternalProvisionError) Error() string {
	if e.Err == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
}

func (e *ChannelExternalProvisionError) Unwrap() error {
	return e.Err
}

func WithChannelExternalProvisioner(p ChannelExternalProvisioner) ChannelHandlerOption {
	return func(h *ChannelHandler) {
		h.externalProvisioner = p
	}
}

// WithChannelEnqueuer wires the async task enqueuer used to schedule
// post-delete cleanup (channel memory deletion).
func WithChannelEnqueuer(enq enqueue.TaskEnqueuer) ChannelHandlerOption {
	return func(h *ChannelHandler) {
		h.enqueuer = enq
	}
}

func channelNameFromCreate(req *channelMutationRequest, source channelSourceFields) string {
	if source.Origin != "external" {
		return normalizeChannelName(cleanStringPtr(req.Name))
	}
	base := firstNonEmpty(source.ExternalResourceName, cleanStringPtr(req.Name), source.ExternalResourceKey)
	return providerPrefixedChannelName(source.ExternalProvider, base)
}

func providerPrefixedChannelName(provider, name string) string {
	provider = normalizeChannelName(provider)
	name = normalizeChannelName(name)
	if provider == "" || name == "" {
		return ""
	}
	prefix := provider + "-"
	if strings.HasPrefix(name, prefix) {
		return name
	}
	return prefix + name
}

func (h *ChannelHandler) prepareExternalChannelCreate(w http.ResponseWriter, r *http.Request, orgID uuid.UUID, source channelSourceFields) (channelSourceFields, bool) {
	if source.Origin != "external" {
		return source, true
	}
	if source.ExternalConnectionID == nil || *source.ExternalConnectionID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "external_connection_id is required for external channels"})
		return source, false
	}
	conn, ok := h.validateExternalChannelConnection(w, r.Context(), orgID, source)
	if !ok {
		return source, false
	}
	if strings.TrimSpace(source.ExternalWorkspaceKey) == "" {
		source.ExternalWorkspaceKey = conn.NangoConnectionID
	}
	if h.externalProvisioner == nil {
		return source, true
	}
	err := h.externalProvisioner.EnsureExternalChannel(r.Context(), orgID, externalProvisionRequest(source))
	if err == nil {
		return source, true
	}
	status, msg := externalProvisionResponse(err)
	writeJSON(w, status, errorResponse{Error: msg})
	return source, false
}

func (h *ChannelHandler) validateExternalChannelConnection(w http.ResponseWriter, ctx context.Context, orgID uuid.UUID, source channelSourceFields) (model.Connection, bool) {
	var conn model.Connection
	err := h.db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Where("connections.id = ? AND connections.org_id = ? AND connections.revoked_at IS NULL", *source.ExternalConnectionID, orgID).
		First(&conn).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "external_connection_id must belong to an active connection in this org"})
		return model.Connection{}, false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load external connection"})
		return model.Connection{}, false
	}
	if conn.Integration.Provider != source.ExternalProvider {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "external_connection_id provider does not match external_provider"})
		return model.Connection{}, false
	}
	return conn, true
}

func externalProvisionRequest(source channelSourceFields) ChannelExternalProvisionRequest {
	connectionID := uuid.Nil
	if source.ExternalConnectionID != nil {
		connectionID = *source.ExternalConnectionID
	}
	return ChannelExternalProvisionRequest{
		Provider:     source.ExternalProvider,
		ConnectionID: connectionID,
		WorkspaceKey: source.ExternalWorkspaceKey,
		ResourceType: source.ExternalResourceType,
		ResourceKey:  source.ExternalResourceKey,
		ResourceName: source.ExternalResourceName,
	}
}

func externalProvisionResponse(err error) (int, string) {
	var provisionErr *ChannelExternalProvisionError
	if errors.As(err, &provisionErr) {
		status := provisionErr.StatusCode
		if status == 0 {
			status = http.StatusBadRequest
		}
		return status, provisionErr.Message
	}
	return http.StatusInternalServerError, "failed to prepare external channel"
}
