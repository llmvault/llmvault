package tasks

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
)

func runtimeMessageFromEvent(session model.Session, event model.SessionEvent, modelDef *agentruntime.ModelConfig) agentruntime.HTTPMessageRequest {
	payload := map[string]any{}
	for key, value := range event.Payload {
		payload[key] = value
	}
	text, _ := payload["text"].(string)
	user, _ := payload["user"].(string)
	display, _ := payload["user_display_name"].(string)
	if user == "" && event.ActorUserID != nil {
		user = event.ActorUserID.String()
	}
	return agentruntime.HTTPMessageRequest{
		Text:            runtimeTextFromPayload(text, payload),
		SessionID:       session.ID.String(),
		User:            strings.TrimSpace(user),
		UserDisplayName: strings.TrimSpace(display),
		ModelDefinition: modelDef,
		Attachments:     anySlice(payload["attachments"]),
		DynamicContext:  stringSlice(payload["dynamic_context"]),
		Raw:             payload,
	}
}

func runtimeTextFromPayload(text string, payload map[string]any) string {
	text = strings.TrimSpace(text)
	parts := make([]string, 0, 3)
	if text != "" {
		parts = append(parts, text)
	}
	if !strings.Contains(text, "<attachment ") {
		if block := runtimeAttachmentBlock(payload["attachments"]); block != "" {
			parts = append(parts, block)
		}
	}
	if !strings.Contains(text, "Code comments to address:") {
		if block := runtimeCodeLineCommentsBlock(payload["code_line_comments"]); block != "" {
			parts = append(parts, block)
		}
	}
	return strings.Join(parts, "\n\n")
}

func runtimeAttachmentBlock(value any) string {
	items := anySlice(value)
	if len(items) == 0 {
		return ""
	}
	tags := make([]string, 0, len(items))
	for _, item := range items {
		record, ok := mapRecord(item)
		if !ok {
			continue
		}
		url := firstRuntimeString(record, "asset_url", "url")
		mimeType := firstRuntimeString(record, "content_type", "mime_type")
		if url == "" || mimeType == "" {
			continue
		}
		filename := firstRuntimeString(record, "filename", "name")
		if filename == "" {
			filename = "attachment"
		}
		description := firstRuntimeString(record, "rendered_description", "description")
		tags = append(tags, fmt.Sprintf("<attachment name=\"%s\" url=\"%s\" mime_type=\"%s\">\n<description>\n%s\n</description>\n</attachment>",
			escapeXMLText(filename),
			escapeXMLText(url),
			escapeXMLText(mimeType),
			escapeXMLText(description),
		))
	}
	return strings.Join(tags, "\n")
}

func runtimeCodeLineCommentsBlock(value any) string {
	items := anySlice(value)
	if len(items) == 0 {
		return ""
	}
	entries := make([]string, 0, len(items))
	for _, item := range items {
		record, ok := mapRecord(item)
		if !ok {
			continue
		}
		body := strings.TrimSpace(firstRuntimeString(record, "body"))
		path := firstRuntimeString(record, "display_path", "path")
		lineNumber := runtimeInt(record["line_number"])
		if body == "" || path == "" || lineNumber <= 0 {
			continue
		}
		location := fmt.Sprintf("%s:%s", path, runtimeLineLabel(lineNumber, firstRuntimeString(record, "side")))
		entries = append(entries, fmt.Sprintf("%d. %s\n%s", len(entries)+1, location, indentRuntimeMessageBody(body)))
	}
	if len(entries) == 0 {
		return ""
	}
	return "Code comments to address:\n\n" + strings.Join(entries, "\n\n")
}

func stringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]string); ok {
			return append([]string(nil), typed...)
		}
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func anySlice(value any) []any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	return append([]any(nil), items...)
}

func mapRecord(value any) (map[string]any, bool) {
	if record, ok := value.(map[string]any); ok {
		return record, true
	}
	return nil, false
}

func firstRuntimeString(record map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := record[key].(string)
		if ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func runtimeInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return 0
}

func runtimeLineLabel(lineNumber int, side string) string {
	switch side {
	case "additions":
		return fmt.Sprintf("R%d", lineNumber)
	case "deletions":
		return fmt.Sprintf("L%d", lineNumber)
	default:
		return strconv.Itoa(lineNumber)
	}
}

func indentRuntimeMessageBody(body string) string {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	for i, line := range lines {
		lines[i] = "   " + line
	}
	return strings.Join(lines, "\n")
}

func escapeXMLText(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(value)
}

func (h *SessionMessageDeliverHandler) markDelivered(ctx context.Context, queue *model.SessionMessageQueue, delivery *agentruntime.HTTPMessageResponse) error {
	now := time.Now()
	updates := map[string]any{
		"status":       "delivered",
		"delivered_at": now,
		"leased_by":    "",
		"leased_until": nil,
		"last_error":   "",
	}
	if delivery != nil {
		updates["runtime_stream_id"] = strings.TrimSpace(delivery.StreamID)
		updates["runtime_stream_url"] = strings.TrimSpace(delivery.StreamURL)
		updates["runtime_trace_id"] = strings.TrimSpace(delivery.TraceID)
		updates["runtime_turn_id"] = strings.TrimSpace(delivery.TurnID)
	}
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.SessionMessageQueue{}).
			Where("id = ?", queue.ID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("mark session message delivered: %w", err)
		}
		sessionUpdates := map[string]any{
			"agent_turn_status":       model.SessionAgentTurnActive,
			"agent_turn_last_outcome": "",
		}
		if delivery != nil {
			sessionUpdates["agent_turn_id"] = strings.TrimSpace(delivery.TurnID)
			sessionUpdates["agent_stream_id"] = strings.TrimSpace(delivery.StreamID)
		}
		if err := tx.Model(&model.Session{}).
			Where("id = ?", queue.SessionID).
			Updates(sessionUpdates).Error; err != nil {
			return fmt.Errorf("record session active turn: %w", err)
		}
		return nil
	})
	return err
}

func (h *SessionMessageDeliverHandler) releaseClaim(ctx context.Context, queueID uuid.UUID, cause error) error {
	msg := ""
	if cause != nil {
		msg = cause.Error()
		if len(msg) > 1000 {
			msg = msg[:1000]
		}
	}
	return h.db.WithContext(ctx).Model(&model.SessionMessageQueue{}).
		Where("id = ? AND status = ?", queueID, "processing").
		Updates(map[string]any{
			"status":       "pending",
			"leased_by":    "",
			"leased_until": nil,
			"last_error":   msg,
		}).Error
}

func (h *SessionMessageDeliverHandler) releaseSessionTurn(ctx context.Context, sessionID uuid.UUID) error {
	return h.db.WithContext(ctx).Model(&model.Session{}).
		Where("id = ?", sessionID).
		Updates(map[string]any{
			"agent_turn_status":       model.SessionAgentTurnIdle,
			"agent_turn_id":           "",
			"agent_stream_id":         "",
			"agent_turn_started_at":   nil,
			"agent_turn_last_outcome": model.SessionAgentTurnOutcomeFailed,
		}).Error
}
