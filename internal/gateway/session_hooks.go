package gateway

import (
	"context"

	"github.com/usehivy/hivy/internal/model"
)

func (s *Service) notifySessionCreated(ctx context.Context, session model.AgentSession, reason, sourceEvent string) {
	if s == nil || s.onSessionCreated == nil {
		return
	}
	s.onSessionCreated(ctx, session, reason, sourceEvent)
}
