package runtimestream

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
)

func eventIdentity(event Event) (identity string, payloadHash string, err error) {
	payloadRaw, err := json.Marshal(event.Payload)
	if err != nil {
		return "", "", fmt.Errorf("marshal runtime event payload for hash: %w", err)
	}
	sum := sha256.Sum256(payloadRaw)
	payloadHash = hex.EncodeToString(sum[:])
	identityRaw, err := json.Marshal(map[string]string{
		"event_id":     event.EventID,
		"event_type":   event.EventType,
		"payload_hash": payloadHash,
	})
	if err != nil {
		return "", "", fmt.Errorf("marshal runtime event identity: %w", err)
	}
	return string(identityRaw), payloadHash, nil
}

func parseAppendScriptResult(raw any) (status string, expectedSeq int64, streamID string, message string, err error) {
	items, ok := raw.([]any)
	if !ok {
		return "", 0, "", "", fmt.Errorf("unexpected append script result type %T", raw)
	}
	if len(items) != 3 && len(items) != 4 {
		return "", 0, "", "", fmt.Errorf("unexpected append script result length %d", len(items))
	}
	status, err = appendResultString(items[0])
	if err != nil {
		return "", 0, "", "", err
	}
	seqText, err := appendResultString(items[1])
	if err != nil {
		return "", 0, "", "", err
	}
	expectedSeq, err = strconv.ParseInt(seqText, 10, 64)
	if err != nil {
		return "", 0, "", "", fmt.Errorf("parse append sequence %q: %w", seqText, err)
	}
	streamID, err = appendResultString(items[2])
	if err != nil {
		return "", 0, "", "", err
	}
	if len(items) == 4 {
		message, err = appendResultString(items[3])
		if err != nil {
			return "", 0, "", "", err
		}
	}
	return status, expectedSeq, streamID, message, nil
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
