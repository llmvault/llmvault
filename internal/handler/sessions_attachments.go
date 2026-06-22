package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type sessionAttachmentRequestError struct {
	status  int
	message string
}

func (e *sessionAttachmentRequestError) Error() string { return e.message }

type storedImageDescription struct {
	DriveAssetID        string         `json:"drive_asset_id"`
	AssetURL            string         `json:"asset_url"`
	Filename            string         `json:"filename"`
	ContentType         string         `json:"content_type"`
	Category            string         `json:"category"`
	Confidence          float64        `json:"confidence"`
	Analysis            map[string]any `json:"analysis"`
	RenderedDescription string         `json:"rendered_description"`
}

func (h *SessionHandler) hydrateSessionMessageAttachmentsForRequest(w http.ResponseWriter, r *http.Request, session model.Session, payload model.JSON) (model.JSON, bool) {
	next, err := h.hydrateSessionMessageAttachments(r.Context(), session, payload)
	if err == nil {
		return next, true
	}
	var reqErr *sessionAttachmentRequestError
	if errors.As(err, &reqErr) {
		writeJSON(w, reqErr.status, errorResponse{Error: reqErr.message})
		return nil, false
	}
	writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load message attachments"})
	return nil, false
}

func (h *SessionHandler) hydrateSessionMessageAttachments(ctx context.Context, session model.Session, payload model.JSON) (model.JSON, error) {
	ids, err := sessionAttachmentIDs(payload["attachment_ids"])
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return payload, nil
	}
	var assets []model.AgentAsset
	if err := h.db.WithContext(ctx).
		Where("id IN ? AND org_id = ? AND agent_id = ?", ids, session.OrgID, session.AgentID).
		Find(&assets).Error; err != nil {
		return nil, fmt.Errorf("load session attachments: %w", err)
	}
	byID := make(map[uuid.UUID]model.AgentAsset, len(assets))
	for _, asset := range assets {
		byID[asset.ID] = asset
	}
	attachments := make([]any, 0, len(ids))
	for _, id := range ids {
		asset, ok := byID[id]
		if !ok {
			return nil, &sessionAttachmentRequestError{status: http.StatusUnprocessableEntity, message: "attachment not found"}
		}
		attachment, err := sessionAttachmentPayload(asset)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, attachment)
	}
	next := cloneModelJSON(payload)
	next["attachment_ids"] = uuidStrings(ids)
	next["attachments"] = attachments
	return next, nil
}

func sessionAttachmentIDs(value any) ([]uuid.UUID, error) {
	items, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]string); ok {
			items = make([]any, len(typed))
			for i, item := range typed {
				items[i] = item
			}
		} else {
			return nil, nil
		}
	}
	ids := make([]uuid.UUID, 0, len(items))
	seen := map[uuid.UUID]struct{}{}
	for _, item := range items {
		raw, ok := item.(string)
		if !ok {
			return nil, &sessionAttachmentRequestError{status: http.StatusBadRequest, message: "attachment_ids must contain only uuid strings"}
		}
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			return nil, &sessionAttachmentRequestError{status: http.StatusBadRequest, message: "attachment_ids must contain only uuid strings"}
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func sessionAttachmentPayload(asset model.AgentAsset) (map[string]any, error) {
	if !isImageContentType(asset.ContentType) {
		return nil, &sessionAttachmentRequestError{status: http.StatusUnprocessableEntity, message: "attachment must be a described image"}
	}
	if asset.Description == nil || len(*asset.Description) == 0 || string(*asset.Description) == "null" {
		return nil, &sessionAttachmentRequestError{status: http.StatusUnprocessableEntity, message: "attachment image description is required"}
	}
	var desc storedImageDescription
	if err := json.Unmarshal([]byte(*asset.Description), &desc); err != nil {
		return nil, &sessionAttachmentRequestError{status: http.StatusUnprocessableEntity, message: "attachment image description is invalid"}
	}
	rendered := strings.TrimSpace(desc.RenderedDescription)
	if rendered == "" {
		return nil, &sessionAttachmentRequestError{status: http.StatusUnprocessableEntity, message: "attachment image description is required"}
	}
	assetURL := firstNonEmptyString(asset.PublicURL, desc.AssetURL)
	analysis := desc.Analysis
	out := map[string]any{
		"drive_asset_id":       asset.ID.String(),
		"asset_url":            assetURL,
		"filename":             firstNonEmptyString(asset.Filename, desc.Filename),
		"content_type":         firstNonEmptyString(asset.ContentType, desc.ContentType),
		"bytes":                asset.Bytes,
		"category":             desc.Category,
		"confidence":           desc.Confidence,
		"rendered_description": rendered,
		"full_description":     rendered,
		"short_description":    strings.TrimSpace(sessionAttachmentString(analysis["summary"])),
		"important_details":    sessionAttachmentValue(analysis["important_details"]),
		"analysis":             analysis,
	}
	return out, nil
}

func sessionAttachmentValue(value any) string {
	return strings.TrimSpace(compactSessionAttachmentValue(value, 0))
}

func compactSessionAttachmentValue(value any, indent int) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	case []any:
		lines := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := compactSessionAttachmentValue(item, indent+2); text != "" {
				lines = append(lines, strings.Repeat(" ", indent)+"- "+indentMultiline(text, indent+2))
			}
		}
		return strings.Join(lines, "\n")
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		lines := make([]string, 0, len(keys))
		for _, key := range keys {
			if text := compactSessionAttachmentValue(typed[key], indent+2); text != "" {
				lines = append(lines, strings.Repeat(" ", indent)+key+": "+indentMultiline(text, indent+len(key)+2))
			}
		}
		return strings.Join(lines, "\n")
	default:
		return fmt.Sprint(typed)
	}
}

func sessionAttachmentString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out
}
