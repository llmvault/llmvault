package handler

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type channelSourceFields struct {
	Origin               string
	ExternalProvider     string
	ExternalConnectionID *uuid.UUID
	ExternalWorkspaceKey string
	ExternalResourceType string
	ExternalResourceKey  string
	ExternalResourceName string
	ExternalResourceURL  string
	ExternalMetadata     model.JSON
}

func channelSourceFromCreate(w http.ResponseWriter, req *channelMutationRequest) (channelSourceFields, bool) {
	source := channelSourceFields{
		Origin:           "native",
		ExternalMetadata: model.JSON{},
	}
	_, ok := applyChannelSourceRequest(w, &source, req, true)
	return source, ok
}

func applyChannelSourceUpdate(w http.ResponseWriter, channel *model.Channel, req *channelMutationRequest, updates map[string]any) bool {
	source := channelSourceFields{
		Origin:               defaultString(channel.Origin, "native"),
		ExternalProvider:     channel.ExternalProvider,
		ExternalConnectionID: channel.ExternalConnectionID,
		ExternalWorkspaceKey: channel.ExternalWorkspaceKey,
		ExternalResourceType: defaultString(channel.ExternalResourceType, "channel"),
		ExternalResourceKey:  channel.ExternalResourceKey,
		ExternalResourceName: channel.ExternalResourceName,
		ExternalResourceURL:  channel.ExternalResourceURL,
		ExternalMetadata:     channel.ExternalMetadata,
	}
	applied, ok := applyChannelSourceRequest(w, &source, req, false)
	if !ok {
		return false
	}
	if !applied {
		return true
	}
	channel.Origin = source.Origin
	channel.ExternalProvider = source.ExternalProvider
	channel.ExternalConnectionID = source.ExternalConnectionID
	channel.ExternalWorkspaceKey = source.ExternalWorkspaceKey
	channel.ExternalResourceType = source.ExternalResourceType
	channel.ExternalResourceKey = source.ExternalResourceKey
	channel.ExternalResourceName = source.ExternalResourceName
	channel.ExternalResourceURL = source.ExternalResourceURL
	channel.ExternalMetadata = source.ExternalMetadata
	updates["origin"] = source.Origin
	updates["external_provider"] = source.ExternalProvider
	updates["external_connection_id"] = source.ExternalConnectionID
	updates["external_workspace_key"] = source.ExternalWorkspaceKey
	updates["external_resource_type"] = source.ExternalResourceType
	updates["external_resource_key"] = source.ExternalResourceKey
	updates["external_resource_name"] = source.ExternalResourceName
	updates["external_resource_url"] = source.ExternalResourceURL
	updates["external_metadata"] = source.ExternalMetadata
	return true
}

func applyChannelSourceRequest(w http.ResponseWriter, source *channelSourceFields, req *channelMutationRequest, force bool) (bool, bool) {
	touched := force || channelSourceTouched(req)
	if !touched {
		return false, true
	}
	if req.Origin != nil {
		source.Origin = strings.ToLower(cleanStringPtr(req.Origin))
	}
	if source.Origin == "" {
		source.Origin = "native"
	}
	if source.Origin != "native" && source.Origin != "external" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "origin must be native or external"})
		return false, false
	}
	applySourceStrings(source, req)
	if source.ExternalResourceType == "" {
		source.ExternalResourceType = defaultExternalChannelResourceType(source.ExternalProvider)
	}
	id, ok := parseOptionalUUIDForRequest(w, req.ExternalConnectionID, "external_connection_id")
	if !ok {
		return false, false
	}
	if req.ExternalConnectionID != nil {
		source.ExternalConnectionID = id
	}
	if hasExternalSourceData(source) {
		source.Origin = "external"
	}
	if source.Origin == "external" && source.ExternalProvider == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "external_provider is required for external channels"})
		return false, false
	}
	if source.Origin == "external" && !validExternalChannelResourceType(source.ExternalProvider, source.ExternalResourceType) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "external_resource_type does not match external_provider"})
		return false, false
	}
	if source.Origin == "external" && source.ExternalResourceKey == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "external_resource_key is required for external channels"})
		return false, false
	}
	if source.Origin == "native" {
		clearExternalSource(source)
	}
	if req.ExternalMetadata != nil {
		source.ExternalMetadata = normalizeJSONPtr(req.ExternalMetadata)
	}
	if source.ExternalMetadata == nil {
		source.ExternalMetadata = model.JSON{}
	}
	return true, true
}

func channelSourceTouched(req *channelMutationRequest) bool {
	return req.Origin != nil || req.ExternalProvider != nil || req.ExternalConnectionID != nil ||
		req.ExternalWorkspaceKey != nil || req.ExternalResourceType != nil ||
		req.ExternalResourceKey != nil || req.ExternalResourceName != nil ||
		req.ExternalResourceURL != nil || req.ExternalMetadata != nil
}

func applySourceStrings(source *channelSourceFields, req *channelMutationRequest) {
	if req.ExternalProvider != nil {
		source.ExternalProvider = strings.ToLower(cleanStringPtr(req.ExternalProvider))
	}
	if req.ExternalWorkspaceKey != nil {
		source.ExternalWorkspaceKey = cleanStringPtr(req.ExternalWorkspaceKey)
	}
	if req.ExternalResourceType != nil {
		source.ExternalResourceType = strings.ToLower(cleanStringPtr(req.ExternalResourceType))
	}
	if req.ExternalResourceKey != nil {
		source.ExternalResourceKey = cleanStringPtr(req.ExternalResourceKey)
	}
	if req.ExternalResourceName != nil {
		source.ExternalResourceName = cleanStringPtr(req.ExternalResourceName)
	}
	if req.ExternalResourceURL != nil {
		source.ExternalResourceURL = cleanStringPtr(req.ExternalResourceURL)
	}
}

func hasExternalSourceData(source *channelSourceFields) bool {
	return source.ExternalProvider != "" || source.ExternalWorkspaceKey != "" ||
		source.ExternalResourceKey != "" || source.ExternalResourceName != "" ||
		source.ExternalResourceURL != "" || source.ExternalConnectionID != nil
}

func clearExternalSource(source *channelSourceFields) {
	source.ExternalProvider = ""
	source.ExternalConnectionID = nil
	source.ExternalWorkspaceKey = ""
	source.ExternalResourceType = "channel"
	source.ExternalResourceKey = ""
	source.ExternalResourceName = ""
	source.ExternalResourceURL = ""
}

func defaultExternalChannelResourceType(provider string) string {
	if provider == "slack" {
		return "slack_channel"
	}
	return "channel"
}

func validExternalChannelResourceType(provider, resourceType string) bool {
	if provider == "slack" {
		return resourceType == "slack_channel"
	}
	return resourceType != ""
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
