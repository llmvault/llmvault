package tasks

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

const vCPUMinuteMilliseconds int64 = 60_000

type SandboxBillingProcessHandler struct {
	db      *gorm.DB
	notices UsageNoticePublisher
}

func NewSandboxBillingProcessHandler(db *gorm.DB, notices UsageNoticePublisher) *SandboxBillingProcessHandler {
	return &SandboxBillingProcessHandler{db: db, notices: notices}
}

func sandboxCreditsForWeightedMilliseconds(weightedMilliseconds int64) int64 {
	if weightedMilliseconds <= 0 {
		return 0
	}
	return weightedMilliseconds / vCPUMinuteMilliseconds
}

func (h *SandboxBillingProcessHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	orgIDs, changedSessions, err := syncSandboxTurnUsage(ctx, h.db, time.Now().UTC())
	if err != nil {
		return err
	}
	if len(orgIDs) == 0 {
		return nil
	}
	batchRefID := "batch_" + ulid.Make().String()
	var debited int64
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, orgID := range orgIDs {
			if err := lockOrgForBilling(tx, orgID); err != nil {
				return err
			}
			var weightedMilliseconds int64
			if err := tx.Raw(`
				SELECT COALESCE(SUM(usage.active_milliseconds * usage.sandbox_vcpu * usage.credits_per_vcpu_minute), 0)
				FROM sandbox_turn_usage usage
				JOIN sessions ON sessions.id = usage.session_id
				LEFT JOIN sandboxes ON sandboxes.id = sessions.sandbox_id
				WHERE usage.org_id = ?
				  AND COALESCE(sandboxes.provider_id, '') <> ?`, orgID, sandbox.ProviderDesktop).Scan(&weightedMilliseconds).Error; err != nil {
				return fmt.Errorf("sum sandbox turn usage for org %s: %w", orgID, err)
			}
			target := sandboxCreditsForWeightedMilliseconds(weightedMilliseconds)
			var alreadyDebited int64
			if err := tx.Raw(`SELECT COALESCE(SUM(-amount), 0) FROM credit_ledger_entries WHERE org_id = ? AND reason = ? AND amount < 0`,
				orgID, billing.ReasonSandboxCompute).Scan(&alreadyDebited).Error; err != nil {
				return fmt.Errorf("sum sandbox debits for org %s: %w", orgID, err)
			}
			delta := target - alreadyDebited
			if delta <= 0 {
				continue
			}
			balance, err := ledgerBalance(tx, orgID)
			if err != nil {
				return err
			}
			if headroom := balance - billing.CreditOverdraftFloor; delta > headroom {
				delta = headroom
			}
			if delta <= 0 {
				continue
			}
			if err := tx.Create(&model.CreditLedgerEntry{
				OrgID: orgID, Amount: -delta, Reason: billing.ReasonSandboxCompute,
				RefType: "sandbox_turn_batch", RefID: batchRefID,
			}).Error; err != nil {
				return fmt.Errorf("debit sandbox turn usage for org %s: %w", orgID, err)
			}
			debited += delta
		}
		return nil
	})
	if err != nil {
		return err
	}
	for sessionID, orgID := range changedSessions {
		if h.notices != nil {
			if err := h.notices.PublishUsageUpdated(ctx, orgID, sessionID); err != nil {
				logging.Capture(ctx, fmt.Errorf("publish sandbox usage.updated notice: %w", err))
			}
		}
	}
	if debited > 0 {
		logging.FromContext(ctx).InfoContext(ctx, "sandbox billing batch processed",
			"orgs", len(orgIDs), "credits_debited", debited)
	}
	return nil
}

type turnUsageCandidate struct {
	OrgID                uuid.UUID
	SessionID            uuid.UUID
	TurnID               string
	SandboxVCPU          int `gorm:"column:sandbox_vcpu"`
	PricingVersion       int
	CreditsPerVCPUMinute int `gorm:"column:credits_per_vcpu_minute"`
	StartedAt            time.Time
	ObservedThrough      time.Time
	EndedAt              *time.Time
}

// syncSandboxTurnUsage materializes durable turn_started/terminal event pairs.
// Open turns advance to observedAt; closed turns stop changing. The upsert only
// accepts a greater cumulative duration, so retries and late events cannot
// reduce or duplicate previously billed compute.
func syncSandboxTurnUsage(ctx context.Context, db *gorm.DB, observedAt time.Time) ([]uuid.UUID, map[uuid.UUID]uuid.UUID, error) {
	var candidates []turnUsageCandidate
	err := db.WithContext(ctx).Raw(`
WITH starts AS (
    SELECT se.org_id, se.session_id,
           COALESCE(NULLIF(se.turn_id, ''), se.payload->>'turn_id') AS turn_id,
           MIN(se.event_at) AS started_at
      FROM session_events se
      LEFT JOIN sandbox_turn_usage u
        ON u.session_id = se.session_id
       AND u.turn_id = COALESCE(NULLIF(se.turn_id, ''), se.payload->>'turn_id')
     WHERE se.event_type = 'turn_started'
       AND (u.session_id IS NULL OR u.ended_at IS NULL)
     GROUP BY se.org_id, se.session_id, COALESCE(NULLIF(se.turn_id, ''), se.payload->>'turn_id')
), terminals AS (
    SELECT se.session_id,
           COALESCE(NULLIF(se.turn_id, ''), se.payload->>'turn_id') AS turn_id,
           MIN(se.event_at) AS ended_at
      FROM session_events se
      JOIN starts st
        ON st.session_id = se.session_id
       AND st.turn_id = COALESCE(NULLIF(se.turn_id, ''), se.payload->>'turn_id')
     WHERE se.event_type IN ('turn_completed', 'turn_failed', 'turn_interrupted')
     GROUP BY se.session_id, COALESCE(NULLIF(se.turn_id, ''), se.payload->>'turn_id')
)
SELECT st.org_id, st.session_id, st.turn_id,
       s.sandbox_vcpu, s.sandbox_pricing_version AS pricing_version,
       s.sandbox_credits_per_vcpu_minute AS credits_per_vcpu_minute,
       st.started_at,
       CASE
         WHEN te.ended_at IS NOT NULL THEN te.ended_at
         WHEN s.agent_turn_status = 'active' AND s.agent_turn_id = st.turn_id THEN ?::timestamptz
         ELSE GREATEST(st.started_at, s.updated_at)
       END AS observed_through,
       CASE
         WHEN te.ended_at IS NOT NULL THEN te.ended_at
         WHEN s.agent_turn_status = 'active' AND s.agent_turn_id = st.turn_id THEN NULL
         ELSE GREATEST(st.started_at, s.updated_at)
       END AS ended_at
  FROM starts st
  JOIN sessions s ON s.id = st.session_id AND s.org_id = st.org_id
  LEFT JOIN sandboxes sb ON sb.id = s.sandbox_id
  LEFT JOIN terminals te ON te.session_id = st.session_id AND te.turn_id = st.turn_id
 WHERE st.turn_id IS NOT NULL AND st.turn_id <> ''
   AND COALESCE(sb.provider_id, '') <> ?
	`, observedAt, sandbox.ProviderDesktop).Scan(&candidates).Error
	if err != nil {
		return nil, nil, fmt.Errorf("select sandbox turn usage: %w", err)
	}
	changedSessions := make(map[uuid.UUID]uuid.UUID, len(candidates))
	for _, candidate := range candidates {
		if candidate.SandboxVCPU <= 0 || candidate.CreditsPerVCPUMinute <= 0 || candidate.ObservedThrough.Before(candidate.StartedAt) {
			continue
		}
		activeMilliseconds := candidate.ObservedThrough.Sub(candidate.StartedAt).Milliseconds()
		row := model.SandboxTurnUsage{
			OrgID: candidate.OrgID, SessionID: candidate.SessionID, TurnID: candidate.TurnID,
			SandboxVCPU: candidate.SandboxVCPU, PricingVersion: candidate.PricingVersion,
			CreditsPerVCPUMinute: candidate.CreditsPerVCPUMinute,
			StartedAt:            candidate.StartedAt.UTC(), ObservedThrough: candidate.ObservedThrough.UTC(),
			EndedAt: candidate.EndedAt, ActiveMilliseconds: activeMilliseconds,
		}
		if err := db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "session_id"}, {Name: "turn_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"observed_through":    gorm.Expr("GREATEST(sandbox_turn_usage.observed_through, EXCLUDED.observed_through)"),
				"ended_at":            gorm.Expr("COALESCE(sandbox_turn_usage.ended_at, EXCLUDED.ended_at)"),
				"active_milliseconds": gorm.Expr("GREATEST(sandbox_turn_usage.active_milliseconds, EXCLUDED.active_milliseconds)"),
				"updated_at":          observedAt,
			}),
		}).Create(&row).Error; err != nil {
			return nil, nil, fmt.Errorf("upsert sandbox turn usage %s/%s: %w", candidate.SessionID, candidate.TurnID, err)
		}
		changedSessions[candidate.SessionID] = candidate.OrgID
	}
	var orgIDs []uuid.UUID
	if err := db.WithContext(ctx).Table("sandbox_turn_usage AS usage").
		Joins("JOIN sessions ON sessions.id = usage.session_id").
		Joins("LEFT JOIN sandboxes ON sandboxes.id = sessions.sandbox_id").
		Where("COALESCE(sandboxes.provider_id, '') <> ?", sandbox.ProviderDesktop).
		Distinct("usage.org_id").Pluck("usage.org_id", &orgIDs).Error; err != nil {
		return nil, nil, fmt.Errorf("list orgs with sandbox turn usage: %w", err)
	}
	sort.Slice(orgIDs, func(i, j int) bool { return orgIDs[i].String() < orgIDs[j].String() })
	return orgIDs, changedSessions, nil
}
