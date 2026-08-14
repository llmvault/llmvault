package handler

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type sessionEventPaginationCursor struct {
	SequenceNumber int64
	ID             uuid.UUID
}

func parseSessionEventPagination(r *http.Request) (int, *sessionEventPaginationCursor, error) {
	limit, err := parsePaginationLimit(r)
	if err != nil {
		return 0, nil, err
	}
	raw := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if raw == "" || raw == "0" {
		return limit, nil, nil
	}
	cursor, err := decodeSessionEventCursor(raw)
	if err != nil {
		return 0, nil, fmt.Errorf("invalid cursor")
	}
	return limit, cursor, nil
}

func applySessionEventPagination(
	query *gorm.DB,
	cursor *sessionEventPaginationCursor,
	limit int,
) *gorm.DB {
	if cursor != nil {
		query = query.Where(
			"(sequence_number, id) < (?, ?)",
			cursor.SequenceNumber,
			cursor.ID,
		)
	}
	return query.Order("sequence_number DESC, id DESC").Limit(limit + 1)
}

func encodeSessionEventCursor(sequenceNumber int64, id uuid.UUID) string {
	raw := "v1|" + strconv.FormatInt(sequenceNumber, 10) + "|" + id.String()
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

func decodeSessionEventCursor(raw string) (*sessionEventPaginationCursor, error) {
	decoded, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 3 || parts[0] != "v1" {
		return nil, fmt.Errorf("malformed session event cursor")
	}
	sequenceNumber, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || sequenceNumber < 0 {
		return nil, fmt.Errorf("invalid session event sequence")
	}
	id, err := uuid.Parse(parts[2])
	if err != nil {
		return nil, err
	}
	return &sessionEventPaginationCursor{SequenceNumber: sequenceNumber, ID: id}, nil
}
