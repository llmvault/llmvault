package tasks

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sheets"
)

// importOptionsError marks a job's options as unusable — terminal, no retry.
type importOptionsError struct{ message string }

func (e *importOptionsError) Error() string {
	return "sheets import: invalid options: " + e.message
}

// csvFormatError marks the uploaded file as unparseable CSV — terminal.
type csvFormatError struct {
	line int64
	err  error
}

func (e *csvFormatError) Error() string {
	if e.line > 0 {
		return fmt.Sprintf("sheets import: malformed CSV near line %d: %v", e.line, e.err)
	}
	return fmt.Sprintf("sheets import: malformed CSV: %v", e.err)
}

func (e *csvFormatError) Unwrap() error { return e.err }

// sheetImportOptions is the parsed shape of sheet_import_jobs.options.
type sheetImportOptions struct {
	hasHeader bool
	delimiter rune
	// fieldMapping maps a CSV column — by header name or 0-based index
	// string — to an existing fld_ id. Non-empty mapping wins over
	// createFields.
	fieldMapping map[string]string
	createFields bool
}

// parseImportOptions validates job options: has_header (default true),
// delimiter (single character, default ','), and either field_mapping
// (columns → existing fld_ ids) or create_fields (infer types; the default
// when no mapping is given).
func parseImportOptions(raw model.JSON) (*sheetImportOptions, error) {
	opts := &sheetImportOptions{hasHeader: true, delimiter: ',', createFields: true}
	if raw == nil {
		return opts, nil
	}
	if v, ok := raw["has_header"]; ok {
		b, isBool := v.(bool)
		if !isBool {
			return nil, &importOptionsError{message: "has_header must be a boolean"}
		}
		opts.hasHeader = b
	}
	if v, ok := raw["delimiter"]; ok {
		s, isString := v.(string)
		if !isString || utf8.RuneCountInString(s) != 1 {
			return nil, &importOptionsError{message: "delimiter must be a single character"}
		}
		r, _ := utf8.DecodeRuneInString(s)
		if r == '\r' || r == '\n' || r == '"' || r == utf8.RuneError {
			return nil, &importOptionsError{message: "delimiter must not be a quote or newline"}
		}
		opts.delimiter = r
	}
	if v, ok := raw["create_fields"]; ok {
		b, isBool := v.(bool)
		if !isBool {
			return nil, &importOptionsError{message: "create_fields must be a boolean"}
		}
		opts.createFields = b
	}
	if v, ok := raw["field_mapping"]; ok && v != nil {
		mapping, isMap := v.(map[string]any)
		if !isMap {
			return nil, &importOptionsError{message: "field_mapping must be an object of column → field id"}
		}
		opts.fieldMapping = make(map[string]string, len(mapping))
		for column, rawID := range mapping {
			fieldID, isString := rawID.(string)
			if !isString || !sheets.ValidFieldID(fieldID) {
				return nil, &importOptionsError{message: fmt.Sprintf("field_mapping[%q] must be a valid field id", column)}
			}
			opts.fieldMapping[column] = fieldID
		}
	}
	if len(opts.fieldMapping) > 0 {
		opts.createFields = false
	} else if !opts.createFields {
		return nil, &importOptionsError{message: "either field_mapping or create_fields is required"}
	}
	return opts, nil
}

// resolveMappedColumns turns a field_mapping into per-column targets. A
// column matches by exact header name first, then by its 0-based index as a
// string. Every mapped field must be an active field on the job's page;
// unmapped columns are skipped.
func resolveMappedColumns(opts *sheetImportOptions, header []string, columnCount int, fields []model.SheetField) ([]string, error) {
	known := make(map[string]bool, len(fields))
	for _, field := range fields {
		known[field.ID] = true
	}
	for column, fieldID := range opts.fieldMapping {
		if !known[fieldID] {
			return nil, &importOptionsError{message: fmt.Sprintf("field_mapping[%q] references unknown field %s", column, fieldID)}
		}
	}
	targets := make([]string, columnCount)
	matched := 0
	for i := 0; i < columnCount; i++ {
		if opts.hasHeader && i < len(header) {
			if fieldID, ok := opts.fieldMapping[strings.TrimSpace(header[i])]; ok {
				targets[i] = fieldID
				matched++
				continue
			}
		}
		if fieldID, ok := opts.fieldMapping[strconv.Itoa(i)]; ok {
			targets[i] = fieldID
			matched++
		}
	}
	if matched == 0 {
		return nil, &importOptionsError{message: "field_mapping matches no CSV columns"}
	}
	return targets, nil
}

// importColumnName names a created column from the header or its position.
func importColumnName(header []string, index int, hasHeader bool) string {
	if hasHeader && index < len(header) {
		if name := strings.TrimSpace(header[index]); name != "" {
			return name
		}
	}
	return fmt.Sprintf("Column %d", index+1)
}
