package sheets

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// cursorPayload is the opaque keyset cursor: the last row's key value and ID
// plus the ordering it was produced under (so a cursor cannot be replayed
// against a different ordering).
type cursorPayload struct {
	K  string `json:"k"`
	V  any    `json:"v"`
	ID string `json:"id"`
	D  bool   `json:"d"`
}

// EncodeCursor builds the next-page cursor from the last row of a result
// under (keyField, keyDesc) ordering.
func EncodeCursor(keyField string, keyDesc bool, row *model.SheetRow) (string, error) {
	payload := cursorPayload{K: keyField, ID: row.ID.String(), D: keyDesc}
	switch keyField {
	case SortPosition:
		payload.V = row.Position
	case SortCreatedAt:
		payload.V = row.CreatedAt.UTC().Format(time.RFC3339Nano)
	default:
		return "", ErrInvalidCursor
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// compileCursor turns an opaque cursor into a keyset row-comparison
// predicate. The cursor must have been produced under the same key field and
// direction as the current query.
func compileCursor(cursor, keyField string, keyDesc bool) (string, []any, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", nil, ErrInvalidCursor
	}
	var payload cursorPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", nil, ErrInvalidCursor
	}
	if payload.K != keyField || payload.D != keyDesc {
		return "", nil, ErrInvalidCursor
	}
	rowID, err := uuid.Parse(payload.ID)
	if err != nil {
		return "", nil, ErrInvalidCursor
	}
	cmp := ">"
	if keyDesc {
		cmp = "<"
	}
	switch keyField {
	case SortPosition:
		pos, ok := payload.V.(float64)
		if !ok {
			return "", nil, ErrInvalidCursor
		}
		return "(position, id) " + cmp + " (?, ?)", []any{pos, rowID}, nil
	case SortCreatedAt:
		s, ok := payload.V.(string)
		if !ok {
			return "", nil, ErrInvalidCursor
		}
		if _, err := time.Parse(time.RFC3339Nano, s); err != nil {
			return "", nil, ErrInvalidCursor
		}
		return "(created_at, id) " + cmp + " (?::timestamptz, ?)", []any{s, rowID}, nil
	default:
		return "", nil, ErrInvalidCursor
	}
}
