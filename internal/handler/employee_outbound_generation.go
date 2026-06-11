package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func (h *EmployeeOutboundWebhookHandler) recordRuntimeModelUsageGeneration(ctx context.Context, sb *model.Sandbox, event *employeeOutboundEvent, payload map[string]any) error {
	if sb == nil || sb.OrgID == nil || sb.EmployeeID == nil {
		return fmt.Errorf("sandbox missing org_id or employee_id")
	}
	modelPayload := runtimeModelUsagePayload(payload)
	usage := mapValue(modelPayload, "usage")
	if len(usage) == 0 {
		return fmt.Errorf("missing usage payload")
	}

	tokenRecord, err := h.runtimeProxyToken(ctx, *sb.OrgID, *sb.EmployeeID, sb.ID)
	if err != nil {
		return err
	}
	providerID, err := h.credentialProviderID(ctx, tokenRecord.CredentialID)
	if err != nil {
		return err
	}

	sessionID := stringValue(payload, "session_id")
	source := employeeEventSource(payload)
	sequence := intValue(payload, "sequence")
	tags := pq.StringArray{"employee-runtime", "source:" + source, "sandbox:" + sb.ID.String()}
	if sessionID != "" {
		tags = append(tags, "session:"+sessionID)
	}
	if sequence > 0 {
		tags = append(tags, fmt.Sprintf("sequence:%d", sequence))
	}

	gen := model.Generation{
		ID:              runtimeGenerationID(sb.ID, sessionID, sequence),
		OrgID:           *sb.OrgID,
		CredentialID:    tokenRecord.CredentialID,
		TokenJTI:        tokenRecord.JTI,
		ProviderID:      providerID,
		Model:           stringValue(modelPayload, "model"),
		RequestPath:     "/v1/proxy/v1/chat/completions",
		IsStreaming:     true,
		InputTokens:     intValue(usage, "prompt_tokens"),
		OutputTokens:    intValue(usage, "completion_tokens"),
		CachedTokens:    intValue(usage, "cached_tokens"),
		ReasoningTokens: intValue(usage, "reasoning_tokens"),
		Cost:            floatValue(usage, "cost"),
		UpstreamStatus:  200,
		UserID:          tokenMetaString(tokenRecord.Meta, "user"),
		Tags:            tags,
		CreatedAt:       event.At.UTC(),
		IsSystem:        true,
	}
	if err := h.db.WithContext(ctx).Create(&gen).Error; err != nil {
		// The generation ID is derived deterministically from
		// (sandbox_id, session_id, sequence) so a runtime outbox retry of the
		// same usage event collides on the primary key. Treat that collision as
		// already-recorded so the retry acks 200 instead of re-failing forever.
		if isDuplicateKeyError(err) {
			return nil
		}
		return err
	}
	return nil
}

// runtimeGenerationID derives a stable generation primary key from the runtime
// usage event's identity tuple so retries of the same billable event are
// idempotent. When the tuple is incomplete (no session/sequence) we fall back to
// a random ULID — those events cannot be safely deduped, and dropping them would
// be worse than a rare duplicate.
func runtimeGenerationID(sandboxID uuid.UUID, sessionID string, sequence int) string {
	if sessionID == "" || sequence <= 0 {
		return "gen_" + ulid.Make().String()
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d", sandboxID.String(), sessionID, sequence)))
	return "gen_rt_" + hex.EncodeToString(sum[:16])
}

func (h *EmployeeOutboundWebhookHandler) runtimeProxyToken(ctx context.Context, orgID, employeeID, sandboxID uuid.UUID) (model.Token, error) {
	var tokenRecord model.Token
	query := h.db.WithContext(ctx).
		Where("org_id = ? AND revoked_at IS NULL AND meta->>'employee_id' = ? AND meta->>'type' = ?",
			orgID, employeeID.String(), "employee_proxy")
	if sandboxID != uuid.Nil {
		query = query.Where("meta->>'sandbox_id' = ?", sandboxID.String())
	}
	err := query.Order("created_at DESC").First(&tokenRecord).Error
	if err == nil {
		return tokenRecord, nil
	}
	if sandboxID == uuid.Nil || !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Token{}, fmt.Errorf("load runtime proxy token: %w", err)
	}
	err = h.db.WithContext(ctx).
		Where("org_id = ? AND revoked_at IS NULL AND meta->>'employee_id' = ? AND meta->>'type' = ?",
			orgID, employeeID.String(), "employee_proxy").
		Order("created_at DESC").
		First(&tokenRecord).Error
	if err != nil {
		return model.Token{}, fmt.Errorf("load runtime proxy token fallback: %w", err)
	}
	return tokenRecord, nil
}

func (h *EmployeeOutboundWebhookHandler) credentialProviderID(ctx context.Context, credentialID uuid.UUID) (string, error) {
	var credential model.Credential
	if err := h.db.WithContext(ctx).Select("provider_id").Where("id = ?", credentialID).First(&credential).Error; err != nil {
		return "", fmt.Errorf("load credential provider: %w", err)
	}
	return credential.ProviderID, nil
}

func runtimeModelUsagePayload(payload map[string]any) map[string]any {
	agentEvent := mapValue(payload, "agent_event")
	return mapValue(agentEvent, "payload")
}

func mapValue(payload map[string]any, key string) map[string]any {
	if value, ok := payload[key].(map[string]any); ok {
		return value
	}
	return nil
}

func intValue(payload map[string]any, key string) int {
	switch value := payload[key].(type) {
	case float64:
		return int(math.Round(value))
	case int:
		return value
	case int64:
		return int(value)
	default:
		return 0
	}
}

func floatValue(payload map[string]any, key string) float64 {
	switch value := payload[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	case int64:
		return float64(value)
	default:
		return 0
	}
}

func tokenMetaString(meta model.JSON, key string) string {
	if value, ok := meta[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}
