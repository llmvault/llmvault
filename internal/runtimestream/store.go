package runtimestream

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/redisutil"
)

const (
	AppendAccepted  = "accepted"
	AppendDuplicate = "duplicate"
	AppendGap       = "gap"
	AppendConflict  = "conflict"
)

var appendScript = redis.NewScript(`
	local last = redis.call("GET", KEYS[1])
	local current = 0
if last then
	current = tonumber(last)
end

	local seq = tonumber(ARGV[1])
	if seq <= current then
		local existing = redis.call("HGET", KEYS[3], tostring(seq))
		if not existing then
			return {"conflict", tostring(current + 1), "", "runtime_seq identity is no longer retained"}
		end
		if existing == ARGV[5] then
			return {"duplicate", tostring(current + 1), "", ""}
		end
		return {"conflict", tostring(current + 1), "", "runtime_seq conflicts with an existing event"}
	end

	if seq ~= current + 1 then
		return {"gap", tostring(current + 1), "", ""}
	end

	local maxlen = tonumber(ARGV[6])
	local stream_id
	if maxlen and maxlen > 0 then
		stream_id = redis.call("XADD", KEYS[2], "MAXLEN", "~", maxlen, "*", "event", ARGV[2], "identity", ARGV[5], "event_id", ARGV[8], "payload_hash", ARGV[9])
	else
		stream_id = redis.call("XADD", KEYS[2], "*", "event", ARGV[2], "identity", ARGV[5], "event_id", ARGV[8], "payload_hash", ARGV[9])
	end
	redis.call("HSET", KEYS[3], tostring(seq), ARGV[5])
	redis.call("SET", KEYS[1], tostring(seq))
	local ttl = tonumber(ARGV[7])
	if ttl and ttl > 0 then
		redis.call("PEXPIRE", KEYS[1], ttl)
		redis.call("PEXPIRE", KEYS[3], ttl)
	end
	redis.call("PUBLISH", ARGV[3], ARGV[4])
	return {"accepted", tostring(seq + 1), stream_id, ""}
	`)

var checkpointProjectedScript = redis.NewScript(`
	local current = redis.call("GET", KEYS[1])
	local seq = tonumber(ARGV[1])
	if (not current) or seq > tonumber(current) then
		redis.call("SET", KEYS[1], ARGV[1])
		local ttl = tonumber(ARGV[2])
		if ttl and ttl > 0 then
			redis.call("PEXPIRE", KEYS[1], ttl)
		end
		return 1
	end
	return 0
	`)

type Store struct {
	client       redis.UniversalClient
	shardCount   int
	streamMaxLen int64
	sessionTTL   time.Duration
	cluster      bool
}

type AppendResult struct {
	Status      string
	RuntimeSeq  int64
	ExpectedSeq int64
	StreamKey   string
	StreamID    string
	Duplicate   bool
	Conflict    bool
	Error       string
}

func NewStore(client redis.UniversalClient, shardCount int) *Store {
	if shardCount <= 0 {
		shardCount = DefaultShardCount
	}
	return &Store{
		client:       client,
		shardCount:   shardCount,
		streamMaxLen: DefaultStreamMaxLen,
		sessionTTL:   DefaultSessionCheckpointTTL,
		cluster:      redisutil.IsCluster(client),
	}
}

func (s *Store) Redis() redis.UniversalClient {
	if s == nil {
		return nil
	}
	return s.client
}

func (s *Store) ShardCount() int {
	if s == nil || s.shardCount <= 0 {
		return DefaultShardCount
	}
	return s.shardCount
}

func (s *Store) SessionCheckpointTTL() time.Duration {
	if s == nil || s.sessionTTL <= 0 {
		return DefaultSessionCheckpointTTL
	}
	return s.sessionTTL
}

func (s *Store) StreamMaxLen() int64 {
	if s == nil || s.streamMaxLen <= 0 {
		return DefaultStreamMaxLen
	}
	return s.streamMaxLen
}

func (s *Store) Append(ctx context.Context, event Event) (AppendResult, error) {
	if s == nil || s.client == nil {
		return AppendResult{}, fmt.Errorf("runtime stream store is not configured")
	}
	event.Normalize()
	if err := event.Validate(); err != nil {
		return AppendResult{}, err
	}
	eventRaw, err := event.Marshal()
	if err != nil {
		return AppendResult{}, err
	}
	identity, payloadHash, err := eventIdentity(event)
	if err != nil {
		return AppendResult{}, err
	}
	liveRaw, err := json.Marshal(LiveMessage{
		Kind:       LiveKindRuntime,
		SessionID:  event.SessionID,
		RuntimeSeq: event.RuntimeSeq,
		Event:      &event,
		Published:  time.Now().UTC(),
	})
	if err != nil {
		return AppendResult{}, fmt.Errorf("marshal live runtime message: %w", err)
	}

	shard := ShardForSession(event.SessionID, s.ShardCount())
	streamKey := s.StreamKey(shard)
	keys := []string{s.LastSeqKey(event.SessionID), streamKey, s.EventIndexKey(event.SessionID)}
	args := []any{
		strconv.FormatInt(event.RuntimeSeq, 10),
		string(eventRaw),
		LiveChannel(event.SessionID),
		string(liveRaw),
		identity,
		strconv.FormatInt(s.StreamMaxLen(), 10),
		strconv.FormatInt(s.SessionCheckpointTTL().Milliseconds(), 10),
		event.EventID,
		payloadHash,
	}
	raw, err := appendScript.Run(ctx, s.client, keys, args...).Result()
	if err != nil {
		return AppendResult{}, fmt.Errorf("append runtime stream event: %w", err)
	}
	status, expectedSeq, streamID, message, err := parseAppendScriptResult(raw)
	if err != nil {
		return AppendResult{}, err
	}
	result := AppendResult{
		Status:      status,
		RuntimeSeq:  event.RuntimeSeq,
		ExpectedSeq: expectedSeq,
		StreamKey:   streamKey,
		StreamID:    streamID,
		Duplicate:   status == AppendDuplicate,
		Conflict:    status == AppendConflict,
		Error:       message,
	}
	switch status {
	case AppendAccepted, AppendDuplicate, AppendGap, AppendConflict:
	default:
		return AppendResult{}, fmt.Errorf("unexpected append status %q", status)
	}
	return result, nil
}

func (s *Store) CheckpointProjected(ctx context.Context, sessionID string, runtimeSeq int64) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("runtime stream store is not configured")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || runtimeSeq <= 0 {
		return nil
	}
	if err := checkpointProjectedScript.Run(
		ctx,
		s.client,
		[]string{s.ProjectedSeqKey(sessionID)},
		strconv.FormatInt(runtimeSeq, 10),
		strconv.FormatInt(s.SessionCheckpointTTL().Milliseconds(), 10),
	).Err(); err != nil {
		return fmt.Errorf("checkpoint projected runtime seq: %w", err)
	}
	return nil
}

func (s *Store) PublishCommitted(ctx context.Context, event model.SessionEvent) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("runtime stream store is not configured")
	}
	view := SessionEventToView(event)
	var runtimeSeq int64
	if event.RuntimeSeq != nil {
		runtimeSeq = *event.RuntimeSeq
	}
	raw, err := json.Marshal(LiveMessage{
		Kind:       LiveKindCommitted,
		SessionID:  event.SessionID.String(),
		RuntimeSeq: runtimeSeq,
		Committed:  &view,
		Published:  time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("marshal committed runtime message: %w", err)
	}
	if err := s.client.Publish(ctx, LiveChannel(event.SessionID.String()), string(raw)).Err(); err != nil {
		return fmt.Errorf("publish committed runtime event: %w", err)
	}
	return nil
}

func (s *Store) PublishNotice(ctx context.Context, sessionID uuid.UUID, n Notice) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("runtime stream store is not configured")
	}
	if n.PublishedAt.IsZero() {
		n.PublishedAt = time.Now().UTC()
	}
	raw, err := json.Marshal(LiveMessage{
		Kind:      LiveKindNotice,
		SessionID: sessionID.String(),
		Notice:    &n,
		Published: n.PublishedAt,
	})
	if err != nil {
		return fmt.Errorf("marshal notice message: %w", err)
	}
	if err := s.client.Publish(ctx, LiveChannel(sessionID.String()), string(raw)).Err(); err != nil {
		return fmt.Errorf("publish session notice: %w", err)
	}
	return nil
}

func (s *Store) EnsureConsumerGroup(ctx context.Context, shard int, group string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("runtime stream store is not configured")
	}
	if strings.TrimSpace(group) == "" {
		group = ProjectorGroup
	}
	err := s.client.XGroupCreateMkStream(ctx, s.StreamKey(shard), group, "0").Err()
	if err == nil || strings.Contains(strings.ToUpper(err.Error()), "BUSYGROUP") {
		return nil
	}
	return fmt.Errorf("ensure runtime stream consumer group %s on shard %d: %w", group, shard, err)
}
