package runtimestream

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/model"
)

const (
	AppendAccepted  = "accepted"
	AppendDuplicate = "duplicate"
	AppendGap       = "gap"
)

var appendScript = redis.NewScript(`
local last = redis.call("GET", KEYS[1])
local current = 0
if last then
	current = tonumber(last)
end

local seq = tonumber(ARGV[1])
if seq <= current then
	return {"duplicate", tostring(current), ""}
end

if seq ~= current + 1 then
	return {"gap", tostring(current + 1), ""}
end

local stream_id = redis.call("XADD", KEYS[2], "*", "event", ARGV[2])
redis.call("SET", KEYS[1], tostring(seq))
redis.call("PUBLISH", ARGV[3], ARGV[4])
return {"accepted", tostring(seq), stream_id}
`)

type Store struct {
	client     *redis.Client
	shardCount int
}

type AppendResult struct {
	Status      string
	RuntimeSeq  int64
	ExpectedSeq int64
	StreamKey   string
	StreamID    string
	Duplicate   bool
}

func NewStore(client *redis.Client, shardCount int) *Store {
	if shardCount <= 0 {
		shardCount = DefaultShardCount
	}
	return &Store{client: client, shardCount: shardCount}
}

func (s *Store) Redis() *redis.Client {
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
	streamKey := StreamKey(shard)
	keys := []string{LastSeqKey(event.SessionID), streamKey}
	args := []any{
		strconv.FormatInt(event.RuntimeSeq, 10),
		string(eventRaw),
		LiveChannel(event.SessionID),
		string(liveRaw),
	}
	raw, err := appendScript.Run(ctx, s.client, keys, args...).Result()
	if err != nil {
		return AppendResult{}, fmt.Errorf("append runtime stream event: %w", err)
	}
	status, seq, streamID, err := parseAppendScriptResult(raw)
	if err != nil {
		return AppendResult{}, err
	}
	result := AppendResult{
		Status:     status,
		RuntimeSeq: event.RuntimeSeq,
		StreamKey:  streamKey,
		StreamID:   streamID,
		Duplicate:  status == AppendDuplicate,
	}
	switch status {
	case AppendAccepted:
		result.ExpectedSeq = seq + 1
	case AppendDuplicate:
		result.ExpectedSeq = seq + 1
	case AppendGap:
		result.ExpectedSeq = seq
	default:
		return AppendResult{}, fmt.Errorf("unexpected append status %q", status)
	}
	return result, nil
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

func (s *Store) EnsureConsumerGroup(ctx context.Context, shard int, group string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("runtime stream store is not configured")
	}
	if strings.TrimSpace(group) == "" {
		group = ProjectorGroup
	}
	err := s.client.XGroupCreateMkStream(ctx, StreamKey(shard), group, "0").Err()
	if err == nil || strings.Contains(strings.ToUpper(err.Error()), "BUSYGROUP") {
		return nil
	}
	return fmt.Errorf("ensure runtime stream consumer group %s on shard %d: %w", group, shard, err)
}

func parseAppendScriptResult(raw any) (status string, seq int64, streamID string, err error) {
	items, ok := raw.([]any)
	if !ok {
		return "", 0, "", fmt.Errorf("unexpected append script result type %T", raw)
	}
	if len(items) != 3 {
		return "", 0, "", fmt.Errorf("unexpected append script result length %d", len(items))
	}
	status, err = appendResultString(items[0])
	if err != nil {
		return "", 0, "", err
	}
	seqText, err := appendResultString(items[1])
	if err != nil {
		return "", 0, "", err
	}
	seq, err = strconv.ParseInt(seqText, 10, 64)
	if err != nil {
		return "", 0, "", fmt.Errorf("parse append sequence %q: %w", seqText, err)
	}
	streamID, err = appendResultString(items[2])
	if err != nil {
		return "", 0, "", err
	}
	return status, seq, streamID, nil
}

func appendResultString(raw any) (string, error) {
	switch v := raw.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		return "", fmt.Errorf("unexpected append result item type %T", raw)
	}
}
