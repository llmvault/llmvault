package runtimestream

import (
	"time"

	"github.com/redis/go-redis/v9"
)

type projectorMessageBatch struct {
	messages  []redis.XMessage
	limit     int
	maxWait   time.Duration
	startedAt time.Time
}

func newProjectorMessageBatch(limit int64, maxWait time.Duration) projectorMessageBatch {
	if limit <= 0 {
		limit = defaultProjectorBatchSize
	}
	return projectorMessageBatch{
		messages: make([]redis.XMessage, 0, int(limit)),
		limit:    int(limit),
		maxWait:  maxWait,
	}
}

func (b *projectorMessageBatch) add(messages []redis.XMessage, now time.Time) {
	if len(messages) == 0 {
		return
	}
	if b.startedAt.IsZero() {
		b.startedAt = now
	}
	b.messages = append(b.messages, messages...)
}

func (b *projectorMessageBatch) empty() bool {
	return b == nil || len(b.messages) == 0
}

func (b *projectorMessageBatch) ready(now time.Time) bool {
	if b.empty() {
		return false
	}
	if len(b.messages) >= b.limit {
		return true
	}
	if b.maxWait <= 0 {
		return true
	}
	return !now.Before(b.startedAt.Add(b.maxWait))
}

func (b *projectorMessageBatch) readBlock(now time.Time, idleBlock time.Duration) time.Duration {
	if b.empty() {
		return idleBlock
	}
	if b.maxWait <= 0 {
		return 0
	}
	return b.startedAt.Add(b.maxWait).Sub(now)
}

func (b *projectorMessageBatch) remaining() int64 {
	if b == nil {
		return defaultProjectorBatchSize
	}
	remaining := b.limit - len(b.messages)
	if remaining <= 0 {
		return 0
	}
	return int64(remaining)
}

func (b *projectorMessageBatch) reset() {
	if b == nil {
		return
	}
	b.messages = b.messages[:0]
	b.startedAt = time.Time{}
}
