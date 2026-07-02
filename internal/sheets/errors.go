package sheets

import (
	"errors"
	"fmt"
)

var (
	// ErrNotFound is returned when a sheet, page, field, view, row, or
	// operation does not exist within the caller's org.
	ErrNotFound = errors.New("sheets: not found")
	// ErrAlreadyReverted is returned when reverting an operation twice.
	ErrAlreadyReverted = errors.New("sheets: operation already reverted")
	// ErrInvalidCursor is returned for malformed or mismatched cursors.
	ErrInvalidCursor = errors.New("sheets: invalid cursor")
	// ErrCursorWithFieldSorts is returned when a cursor is combined with
	// field-based sorts; keyset pagination only supports position/created_at.
	ErrCursorWithFieldSorts = errors.New("sheets: cursor pagination is not supported with field sorts")
)

// UnknownFieldError is returned when a filter, sort, or cell references a
// field ID that is not defined on the page.
type UnknownFieldError struct {
	FieldID string
}

func (e *UnknownFieldError) Error() string {
	return fmt.Sprintf("sheets: unknown field %q", e.FieldID)
}

// UnknownFieldTypeError is returned for a field type outside the registry.
type UnknownFieldTypeError struct {
	Type string
}

func (e *UnknownFieldTypeError) Error() string {
	return fmt.Sprintf("sheets: unknown field type %q", e.Type)
}

// UnsupportedOpError is returned when a filter op is not whitelisted for the
// field's type (or is not a known op at all).
type UnsupportedOpError struct {
	FieldID   string
	FieldType string
	Op        string
}

func (e *UnsupportedOpError) Error() string {
	return fmt.Sprintf("sheets: op %q is not supported for field %s of type %s", e.Op, e.FieldID, e.FieldType)
}

// ValueError is returned when a cell or filter value cannot be coerced to the
// field's type.
type ValueError struct {
	FieldID   string
	FieldType string
	Message   string
}

func (e *ValueError) Error() string {
	return fmt.Sprintf("sheets: invalid value for field %s (%s): %s", e.FieldID, e.FieldType, e.Message)
}

// OptionsError is returned when field options fail validation.
type OptionsError struct {
	FieldType string
	Message   string
}

func (e *OptionsError) Error() string {
	return fmt.Sprintf("sheets: invalid options for field type %s: %s", e.FieldType, e.Message)
}

// LimitError is returned when a write exceeds one of the production limits.
type LimitError struct {
	Limit  string
	Max    int
	Actual int
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("sheets: %s limit exceeded: %d > %d", e.Limit, e.Actual, e.Max)
}

// RelationError is returned when relation links fail validation — the target
// page is missing, belongs to another org, or linked rows are not in the
// field's configured target page.
type RelationError struct {
	FieldID string
	Message string
}

func (e *RelationError) Error() string {
	return fmt.Sprintf("sheets: invalid relation value for field %s: %s", e.FieldID, e.Message)
}

// AttachmentError is returned when attachment object keys fail validation
// (not owned by the org, or malformed).
type AttachmentError struct {
	FieldID string
	Key     string
	Message string
}

func (e *AttachmentError) Error() string {
	return fmt.Sprintf("sheets: invalid attachment for field %s: %s", e.FieldID, e.Message)
}
