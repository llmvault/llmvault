package runtimestream

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
	"github.com/usehivy/hivy/internal/tasks"
)

const (
	defaultProjectorBatchSize = int64(100)
	defaultProjectorBlock     = 5 * time.Second
	defaultProjectorMinIdle   = 30 * time.Second
	defaultShardLeaseTTL      = 15 * time.Second
)

var renewLeaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	redis.call("PEXPIRE", KEYS[1], ARGV[2])
	return 1
end
return 0
`)

var releaseLeaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

type Projector struct {
	store     *Store
	db        *gorm.DB
	consumer  string
	batchSize int64
	block     time.Duration
	minIdle   time.Duration
	leaseTTL  time.Duration
	logger    *slog.Logger
	enqueuer  enqueue.TaskEnqueuer
}

func NewProjector(store *Store, db *gorm.DB, logger *slog.Logger) *Projector {
	if logger == nil {
		logger = slog.Default()
	}
	hostname, _ := os.Hostname()
	consumer := fmt.Sprintf("%s-%d", defaultString(hostname, "worker"), os.Getpid())
	return &Projector{
		store:     store,
		db:        db,
		consumer:  consumer,
		batchSize: defaultProjectorBatchSize,
		block:     defaultProjectorBlock,
		minIdle:   defaultProjectorMinIdle,
		leaseTTL:  defaultShardLeaseTTL,
		logger:    logger,
	}
}

func (p *Projector) WithEnqueuer(enqueuer enqueue.TaskEnqueuer) *Projector {
	if p == nil {
		return nil
	}
	p.enqueuer = enqueuer
	return p
}

func (p *Projector) Run(ctx context.Context) error {
	if p == nil || p.store == nil || p.store.Redis() == nil || p.db == nil {
		return fmt.Errorf("runtime stream projector is not configured")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, p.store.ShardCount())
	for shard := range p.store.ShardCount() {
		shard := shard
		go func() {
			errCh <- p.runShard(ctx, shard)
		}()
	}
	for range p.store.ShardCount() {
		if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
			cancel()
			return err
		}
	}
	return ctx.Err()
}

func (p *Projector) runShard(ctx context.Context, shard int) error {
	leaseKey := shardLeaseKey(shard)
	for ctx.Err() == nil {
		ok, err := p.store.Redis().SetNX(ctx, leaseKey, p.consumer, p.leaseTTL).Result()
		if err != nil {
			p.logger.ErrorContext(ctx, "runtime stream projector: acquire shard lease", "shard", shard, "error", err)
			sleepOrDone(ctx, time.Second)
			continue
		}
		if !ok {
			sleepOrDone(ctx, 2*time.Second)
			continue
		}
		p.logger.InfoContext(ctx, "runtime stream projector: acquired shard", "shard", shard, "consumer", p.consumer)
		if err := p.runOwnedShard(ctx, shard, leaseKey); err != nil && !errors.Is(err, context.Canceled) {
			p.logger.ErrorContext(ctx, "runtime stream projector: shard stopped", "shard", shard, "error", err)
		}
		_, _ = releaseLeaseScript.Run(context.WithoutCancel(ctx), p.store.Redis(), []string{leaseKey}, p.consumer).Result()
		sleepOrDone(ctx, 500*time.Millisecond)
	}
	return ctx.Err()
}

func (p *Projector) runOwnedShard(ctx context.Context, shard int, leaseKey string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go p.renewShardLease(ctx, cancel, shard, leaseKey)
	if err := p.store.EnsureConsumerGroup(ctx, shard, ProjectorGroup); err != nil {
		return err
	}
	stream := StreamKey(shard)
	for ctx.Err() == nil {
		if err := p.claimStale(ctx, stream, shard); err != nil {
			p.logger.ErrorContext(ctx, "runtime stream projector: claim stale messages", "shard", shard, "error", err)
			sleepOrDone(ctx, time.Second)
			continue
		}
		streams, err := p.store.Redis().XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    ProjectorGroup,
			Consumer: p.consumer,
			Streams:  []string{stream, ">"},
			Count:    p.batchSize,
			Block:    p.block,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			}
			return err
		}
		for _, result := range streams {
			if err := p.processMessages(ctx, stream, result.Messages); err != nil {
				p.logger.ErrorContext(ctx, "runtime stream projector: process messages", "shard", shard, "error", err)
				sleepOrDone(ctx, time.Second)
				break
			}
		}
	}
	return ctx.Err()
}

func (p *Projector) renewShardLease(ctx context.Context, cancel context.CancelFunc, shard int, leaseKey string) {
	ticker := time.NewTicker(p.leaseTTL / 3)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ok, err := renewLeaseScript.Run(ctx, p.store.Redis(), []string{leaseKey}, p.consumer, strconv.FormatInt(p.leaseTTL.Milliseconds(), 10)).Int()
			if err != nil || ok != 1 {
				p.logger.WarnContext(ctx, "runtime stream projector: lost shard lease", "shard", shard, "error", err)
				cancel()
				return
			}
		}
	}
}

func (p *Projector) claimStale(ctx context.Context, stream string, shard int) error {
	messages, _, err := p.store.Redis().XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   stream,
		Group:    ProjectorGroup,
		Consumer: p.consumer,
		MinIdle:  p.minIdle,
		Start:    "0-0",
		Count:    p.batchSize,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil
		}
		return err
	}
	if len(messages) == 0 {
		return nil
	}
	p.logger.InfoContext(ctx, "runtime stream projector: claimed pending messages", "shard", shard, "count", len(messages))
	return p.processMessages(ctx, stream, messages)
}

func (p *Projector) processMessages(ctx context.Context, stream string, messages []redis.XMessage) error {
	if len(messages) == 0 {
		return nil
	}
	ackIDs := make([]string, 0, len(messages))
	durable := make([]Event, 0, len(messages))
	for _, msg := range messages {
		event, err := EventFromStreamValues(msg.Values)
		if err != nil {
			return fmt.Errorf("decode stream message %s: %w", msg.ID, err)
		}
		ackIDs = append(ackIDs, msg.ID)
		if event.Durability == DurabilityDurable {
			durable = append(durable, event)
		}
	}
	if len(durable) > 0 {
		if err := p.persistDurableEvents(ctx, durable); err != nil {
			return err
		}
	}
	if len(ackIDs) > 0 {
		if err := p.store.Redis().XAck(ctx, stream, ProjectorGroup, ackIDs...).Err(); err != nil {
			return fmt.Errorf("ack runtime stream messages: %w", err)
		}
	}
	return nil
}

func (p *Projector) persistDurableEvents(ctx context.Context, events []Event) error {
	rows := make([]model.SessionEvent, 0, len(events))
	for _, event := range events {
		row, err := sessionEventFromRuntimeEvent(event)
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil
	}
	if err := p.db.WithContext(ctx).Clauses(runtimeSeqOnConflict()).Create(&rows).Error; err != nil {
		return fmt.Errorf("insert durable runtime events: %w", err)
	}
	for _, event := range events {
		if err := p.publishCommittedEvent(ctx, event); err != nil {
			return err
		}
		if err := p.projectSessionState(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (p *Projector) publishCommittedEvent(ctx context.Context, event Event) error {
	sessionID, err := uuid.Parse(event.SessionID)
	if err != nil {
		return err
	}
	var stored model.SessionEvent
	if err := p.db.WithContext(ctx).
		Where("session_id = ? AND runtime_seq = ?", sessionID, event.RuntimeSeq).
		First(&stored).Error; err != nil {
		return fmt.Errorf("load committed runtime event: %w", err)
	}
	return p.store.PublishCommitted(ctx, stored)
}

func (p *Projector) projectSessionState(ctx context.Context, event Event) error {
	sessionID, err := uuid.Parse(event.SessionID)
	if err != nil {
		return err
	}
	updates := map[string]any{
		"updated_at": event.OccurredAt,
	}
	switch event.EventType {
	case runtimeevents.EventTurnStarted:
		updates["agent_turn_status"] = model.SessionAgentTurnActive
		updates["agent_turn_id"] = event.TurnID
		updates["agent_stream_id"] = defaultString(event.StreamID, event.TurnID)
		updates["agent_turn_last_outcome"] = ""
		if !event.OccurredAt.IsZero() {
			updates["agent_turn_started_at"] = event.OccurredAt
		}
	case runtimeevents.EventTurnCompleted, runtimeevents.EventFinal, runtimeevents.EventDone:
		updates["agent_turn_status"] = model.SessionAgentTurnIdle
		updates["agent_turn_id"] = ""
		updates["agent_stream_id"] = ""
		updates["agent_turn_started_at"] = nil
		updates["agent_turn_last_outcome"] = model.SessionAgentTurnOutcomeDone
	case runtimeevents.EventTurnFailed, runtimeevents.EventError, runtimeevents.EventAgentError:
		updates["agent_turn_status"] = model.SessionAgentTurnIdle
		updates["agent_turn_id"] = ""
		updates["agent_stream_id"] = ""
		updates["agent_turn_started_at"] = nil
		updates["agent_turn_last_outcome"] = model.SessionAgentTurnOutcomeFailed
	}
	if len(updates) == 1 && event.EventType != runtimeevents.EventFinal {
		return nil
	}
	if err := p.db.WithContext(ctx).Model(&model.Session{}).
		Where("id = ?", sessionID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("project session state: %w", err)
	}
	if isTerminalRuntimeEvent(event.EventType) {
		if err := p.enqueuePendingSessionDelivery(ctx, sessionID); err != nil {
			return err
		}
	}
	return nil
}

func isTerminalRuntimeEvent(eventType string) bool {
	switch eventType {
	case runtimeevents.EventTurnCompleted, runtimeevents.EventFinal, runtimeevents.EventDone,
		runtimeevents.EventTurnFailed, runtimeevents.EventError, runtimeevents.EventAgentError:
		return true
	default:
		return false
	}
}

func (p *Projector) enqueuePendingSessionDelivery(ctx context.Context, sessionID uuid.UUID) error {
	if p.enqueuer == nil {
		return nil
	}
	var pending int64
	if err := p.db.WithContext(ctx).Model(&model.SessionMessageQueue{}).
		Where("session_id = ? AND status <> ?", sessionID, "delivered").
		Count(&pending).Error; err != nil {
		return fmt.Errorf("count pending session message commands: %w", err)
	}
	if pending == 0 {
		return nil
	}
	if err := tasks.EnqueueSessionMessageDeliver(ctx, p.enqueuer, sessionID); err != nil {
		return fmt.Errorf("enqueue pending session message command: %w", err)
	}
	return nil
}

func sessionEventFromRuntimeEvent(event Event) (model.SessionEvent, error) {
	event.Normalize()
	if err := event.Validate(); err != nil {
		return model.SessionEvent{}, err
	}
	sessionID, err := uuid.Parse(event.SessionID)
	if err != nil {
		return model.SessionEvent{}, fmt.Errorf("parse session id: %w", err)
	}
	orgID, err := uuid.Parse(event.OrgID)
	if err != nil {
		return model.SessionEvent{}, fmt.Errorf("parse org id: %w", err)
	}
	agentID, err := uuid.Parse(event.AgentID)
	if err != nil {
		return model.SessionEvent{}, fmt.Errorf("parse agent id: %w", err)
	}
	var sandboxID *uuid.UUID
	if strings.TrimSpace(event.SandboxID) != "" {
		id, err := uuid.Parse(event.SandboxID)
		if err != nil {
			return model.SessionEvent{}, fmt.Errorf("parse sandbox id: %w", err)
		}
		sandboxID = &id
	}
	var actorUserID *uuid.UUID
	if strings.TrimSpace(event.ActorUserID) != "" {
		id, err := uuid.Parse(event.ActorUserID)
		if err != nil {
			return model.SessionEvent{}, fmt.Errorf("parse actor user id: %w", err)
		}
		actorUserID = &id
	}
	runtimeSeq := event.RuntimeSeq
	payload := model.JSON{}
	for key, value := range event.Payload {
		payload[key] = value
	}
	return model.SessionEvent{
		OrgID:            orgID,
		SessionID:        sessionID,
		AgentID:          agentID,
		SandboxID:        sandboxID,
		RuntimeSessionID: event.SessionID,
		EventID:          event.EventID,
		EventType:        event.EventType,
		ActorUserID:      actorUserID,
		Source:           defaultString(event.Source, "runtime"),
		SequenceNumber:   event.RuntimeSeq,
		RuntimeSeq:       &runtimeSeq,
		RuntimeEventID:   event.EventID,
		TurnID:           event.TurnID,
		SpanID:           event.SpanID,
		Durability:       event.Durability,
		Payload:          payload,
		EventAt:          event.OccurredAt,
	}, nil
}

func runtimeSeqOnConflict() clause.OnConflict {
	return clause.OnConflict{
		Columns:     []clause.Column{{Name: "session_id"}, {Name: "runtime_seq"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{gorm.Expr("runtime_seq IS NOT NULL")}},
		DoNothing:   true,
	}
}

func shardLeaseKey(shard int) string {
	return StreamKey(shard) + ":lease"
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func sleepOrDone(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
