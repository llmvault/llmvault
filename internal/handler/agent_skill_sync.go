package handler

import (
	"context"
	"errors"

	"github.com/usehivy/hivy/internal/model"
)

func (h *AgentOutboundWebhookHandler) syncSkillEvent(ctx context.Context, sb *model.Sandbox, raw map[string]any) error {
	return errors.New("standalone skill sync is disabled; skills must be shipped through plugins")
}
