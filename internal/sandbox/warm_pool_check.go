package sandbox

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

type WarmSlotCheckResult struct {
	Ready   bool
	Pending bool
}

func (p *WarmPool) CheckWarmSlot(ctx context.Context, slotID uuid.UUID) (*WarmSlotCheckResult, error) {
	if p == nil {
		return &WarmSlotCheckResult{}, nil
	}
	var slot model.SandboxWarmSlot
	if err := p.db.WithContext(ctx).First(&slot, "id = ?", slotID).Error; err != nil {
		return nil, err
	}
	if slot.Status != model.SandboxWarmSlotStatusWarming {
		return &WarmSlotCheckResult{}, nil
	}
	status, err := p.provider.GetStatus(ctx, slot.ExternalID)
	if err != nil {
		return nil, err
	}
	switch status {
	case StatusRunning:
	case StatusCreating, StatusStarting:
		return &WarmSlotCheckResult{Pending: true}, nil
	case StatusStopped, StatusArchived, StatusArchiving, StatusError:
		msg := fmt.Sprintf("provider status %s", status)
		if err := p.MarkError(ctx, slot.ID, msg); err != nil {
			return nil, err
		}
		return &WarmSlotCheckResult{}, fmt.Errorf("warm slot %s entered %s", slot.ExternalID, status)
	default:
		return &WarmSlotCheckResult{Pending: true}, nil
	}
	if err := checkWarmSlotHealth(ctx, slot.EndpointURL); err != nil {
		return &WarmSlotCheckResult{Pending: true}, nil
	}
	if err := p.db.WithContext(ctx).Model(&slot).Updates(map[string]any{
		"status":        model.SandboxWarmSlotStatusWarm,
		"error_message": nil,
	}).Error; err != nil {
		return nil, err
	}
	logging.FromContext(ctx).InfoContext(ctx, "sandbox warm slot ready",
		"provider", p.provider.ID(), "mode", slot.Mode, "external_id", slot.ExternalID,
		"endpoint_url", slot.EndpointURL)
	return &WarmSlotCheckResult{Ready: true}, nil
}

func checkWarmSlotHealth(ctx context.Context, endpoint string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	url := strings.TrimRight(endpoint, "/") + "/healthz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz returned %s", resp.Status)
	}
	return nil
}
