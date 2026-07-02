package sheets

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// coerceCells validates and normalizes one row's incoming cell payload:
// every key must be a known active field ID, every value passes the field
// type's coercion, attachment keys must be org-owned, and relation links are
// queued on batch for one batched existence/org/page check. A nil coerced
// value means "clear the cell".
func coerceCells(orgID uuid.UUID, fields map[string]*model.SheetField, input map[string]any, batch *relationBatch) (map[string]any, error) {
	out := make(map[string]any, len(input))
	for key, raw := range input {
		def, ok := fields[key]
		if !ok {
			return nil, &UnknownFieldError{FieldID: key}
		}
		coerced, err := CoerceValue(def, raw)
		if err != nil {
			return nil, err
		}
		if coerced == nil {
			out[key] = nil
			continue
		}
		switch def.Type {
		case FieldTypeAttachment:
			keys, _ := coerced.([]string)
			if err := validateAttachmentKeys(orgID, def.ID, keys); err != nil {
				return nil, err
			}
		case FieldTypeRelation:
			ids, _ := coerced.([]string)
			batch.add(def, ids)
		}
		if err := checkCellSize(def.ID, coerced); err != nil {
			return nil, err
		}
		out[key] = coerced
	}
	return out, nil
}

func checkCellSize(fieldID string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("sheets: encode cell %s: %w", fieldID, err)
	}
	if len(encoded) > MaxCellValueBytes {
		return &LimitError{Limit: "cell value bytes", Max: MaxCellValueBytes, Actual: len(encoded)}
	}
	return nil
}

func checkRowSize(data map[string]any) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("sheets: encode row data: %w", err)
	}
	if len(encoded) > MaxRowDataBytes {
		return &LimitError{Limit: "row data bytes", Max: MaxRowDataBytes, Actual: len(encoded)}
	}
	return nil
}

// mergeCells applies a partial update: keys with nil values are removed,
// everything else overwrites; untouched keys are preserved. It returns the
// merged data and the inverse patch (each changed key's prior value, nil for
// keys that did not exist).
func mergeCells(existing model.JSON, incoming map[string]any) (map[string]any, map[string]any) {
	merged := make(map[string]any, len(existing)+len(incoming))
	for key, value := range existing {
		merged[key] = value
	}
	inverse := make(map[string]any, len(incoming))
	for key, value := range incoming {
		if prior, ok := merged[key]; ok {
			inverse[key] = prior
		} else {
			inverse[key] = nil
		}
		if value == nil {
			delete(merged, key)
			continue
		}
		merged[key] = value
	}
	return merged, inverse
}

// stripNilCells removes explicit nil cells from an insert payload so stored
// rows never carry JSON nulls for "empty".
func stripNilCells(data map[string]any) map[string]any {
	for key, value := range data {
		if value == nil {
			delete(data, key)
		}
	}
	return data
}
