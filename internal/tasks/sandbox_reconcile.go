package tasks

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/sandbox"
)

// SandboxReconcileHandler re-syncs our sandbox status mirror with the control
// plane. A gateway-driven wake flips a sandbox to running there without telling
// the Go API, stranding it outside auto-sleep (which keys off our stale status).
type SandboxReconcileHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
}

func NewSandboxReconcileHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator) *SandboxReconcileHandler {
	return &SandboxReconcileHandler{db: db, orchestrator: orchestrator}
}

func (h *SandboxReconcileHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	if h.orchestrator == nil {
		return nil
	}
	provider := h.orchestrator.Provider()
	lister, ok := provider.(sandbox.SandboxStateLister)
	if !ok {
		return nil // docker/local don't diverge the way the control plane does
	}
	states, err := lister.ListSandboxStates(ctx)
	if err != nil {
		return err
	}
	reconciled, err := reconcileSandboxStates(ctx, h.db, provider.ID(), states)
	if err != nil {
		return err
	}
	if err := mirrorGatewayActivity(ctx, h.db, provider.ID(), states); err != nil {
		return err
	}
	if reconciled > 0 {
		logging.FromContext(ctx).InfoContext(ctx, "sandbox reconcile",
			"provider", provider.ID(), "states", len(states), "reconciled", reconciled)
	}
	return nil
}

// mirrorGatewayActivity copies the control plane's last-gateway-activity
// timestamp onto our rows — the fallback signal the auto-sleep app-branch uses
// for apps that predate the in-app ping. Null timestamps are dropped so the
// array casts cleanly.
func mirrorGatewayActivity(ctx context.Context, db *gorm.DB, providerID string, states []sandbox.SandboxState) error {
	ids := make([]string, 0, len(states))
	tss := make([]string, 0, len(states))
	for _, st := range states {
		if st.ExternalID == "" || st.LastGatewayActivityAt == nil {
			continue
		}
		ids = append(ids, st.ExternalID)
		tss = append(tss, st.LastGatewayActivityAt.UTC().Format(time.RFC3339Nano))
	}
	if len(ids) == 0 {
		return nil
	}
	return db.WithContext(ctx).Exec(`
UPDATE sandboxes AS s
   SET last_gateway_activity_at = v.gw::timestamptz
  FROM unnest(?::text[], ?::text[]) AS v(external_id, gw)
 WHERE s.provider_id = ? AND s.external_id = v.external_id`,
		pq.StringArray(ids), pq.StringArray(tss), providerID).Error
}

// reconcileSandboxStates bulk-corrects mirrored status in one UPDATE. It only
// touches the running/stopped pair on both sides — rows mid-transition
// (creating/error/archiving) are Go-API-driven and must not be clobbered.
func reconcileSandboxStates(ctx context.Context, db *gorm.DB, providerID string, states []sandbox.SandboxState) (int64, error) {
	ids := make([]string, 0, len(states))
	statuses := make([]string, 0, len(states))
	for _, st := range states {
		if st.ExternalID == "" {
			continue
		}
		if st.Status != sandbox.StatusRunning && st.Status != sandbox.StatusStopped {
			continue
		}
		ids = append(ids, st.ExternalID)
		statuses = append(statuses, string(st.Status))
	}
	if len(ids) == 0 {
		return 0, nil
	}
	res := db.WithContext(ctx).Exec(`
UPDATE sandboxes AS s
   SET status = v.status, updated_at = now()
  FROM unnest(?::text[], ?::text[]) AS v(external_id, status)
 WHERE s.provider_id = ?
   AND s.external_id = v.external_id
   AND s.status IN ('running', 'stopped')
   AND s.status <> v.status`,
		pq.StringArray(ids), pq.StringArray(statuses), providerID)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}
