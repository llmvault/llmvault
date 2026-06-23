package runtimestream

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sourcegraph/conc/pool"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/goroutine"
)

const (
	defaultProjectorBatchSize = int64(100)
	defaultProjectorBlock     = 5 * time.Second
	defaultProjectorFlushWait = time.Second
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
	flushWait time.Duration
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
		flushWait: defaultProjectorFlushWait,
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

	shards := pool.New().
		WithContext(ctx).
		WithCancelOnError().
		WithFirstError().
		WithMaxGoroutines(p.store.ShardCount())
	for shard := range p.store.ShardCount() {
		shards.Go(func(ctx context.Context) error {
			return p.runShard(ctx, shard)
		})
	}
	if err := shards.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		cancel()
		return err
	}
	return ctx.Err()
}

func (p *Projector) runShard(ctx context.Context, shard int) error {
	leaseKey := shardLeaseKey(shard)
	for ctx.Err() == nil {
		res, err := p.store.Redis().SetArgs(ctx, leaseKey, p.consumer, redis.SetArgs{Mode: "NX", TTL: p.leaseTTL}).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			p.logger.ErrorContext(ctx, "runtime stream projector: acquire shard lease", "shard", shard, "error", err)
			sleepOrDone(ctx, time.Second)
			continue
		}
		if res != "OK" {
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

	goroutine.Go(ctx, func(ctx context.Context) {
		p.renewShardLease(ctx, cancel, shard, leaseKey)
	})
	if err := p.store.EnsureConsumerGroup(ctx, shard, ProjectorGroup); err != nil {
		return err
	}
	stream := StreamKey(shard)
	batch := newProjectorMessageBatch(p.batchSize, p.flushWait)
	for ctx.Err() == nil {
		if batch.empty() {
			if err := p.claimStale(ctx, stream, shard); err != nil {
				p.logger.ErrorContext(ctx, "runtime stream projector: claim stale messages", "shard", shard, "error", err)
				sleepOrDone(ctx, time.Second)
				continue
			}
		}
		now := time.Now()
		if batch.ready(now) {
			p.flushRuntimeBatch(ctx, stream, shard, &batch)
			continue
		}
		block := batch.readBlock(now, p.block)
		if block <= 0 {
			p.flushRuntimeBatch(ctx, stream, shard, &batch)
			continue
		}
		streams, err := p.store.Redis().XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    ProjectorGroup,
			Consumer: p.consumer,
			Streams:  []string{stream, ">"},
			Count:    batch.remaining(),
			Block:    block,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				if !batch.empty() {
					p.flushRuntimeBatch(ctx, stream, shard, &batch)
				}
				continue
			}
			return err
		}
		for _, result := range streams {
			batch.add(result.Messages, time.Now())
			if batch.ready(time.Now()) {
				p.flushRuntimeBatch(ctx, stream, shard, &batch)
			}
		}
	}
	return ctx.Err()
}

func (p *Projector) flushRuntimeBatch(ctx context.Context, stream string, shard int, batch *projectorMessageBatch) {
	if batch == nil || batch.empty() {
		return
	}
	if err := p.processMessages(ctx, stream, batch.messages); err != nil {
		p.logger.ErrorContext(ctx, "runtime stream projector: process messages", "shard", shard, "error", err)
		sleepOrDone(ctx, time.Second)
	}
	batch.reset()
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
