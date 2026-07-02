package tasks

import (
	"strings"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sheets"
)

// importInferenceSampleRows caps how many data rows are buffered for type
// inference before streaming resumes (plan §2: first 1,000 rows).
const importInferenceSampleRows = 1000

// inferCandidates orders detection: a column only keeps a candidate type
// while every non-empty sampled value coerces to it, and the first survivor
// wins. Checkbox is restricted to explicit true/false/yes/no so numeric 0/1
// columns infer as number; everything else falls back to text.
var inferCandidates = []string{
	sheets.FieldTypeCheckbox,
	sheets.FieldTypeNumber,
	sheets.FieldTypeDate,
	sheets.FieldTypeEmail,
	sheets.FieldTypeURL,
}

// inferColumnTypes returns one field type per column, inferred from the
// sampled rows via the sheets type registry's own coercion rules.
func inferColumnTypes(sample [][]string, columnCount int) []string {
	types := make([]string, columnCount)
	for col := 0; col < columnCount; col++ {
		types[col] = inferColumnType(sample, col)
	}
	return types
}

func inferColumnType(sample [][]string, col int) string {
	alive := make(map[string]bool, len(inferCandidates))
	for _, candidate := range inferCandidates {
		alive[candidate] = true
	}
	sawValue := false
	for _, record := range sample {
		if col >= len(record) {
			continue
		}
		value := strings.TrimSpace(record[col])
		if value == "" {
			continue
		}
		sawValue = true
		for candidate := range alive {
			if !valueMatchesType(candidate, value) {
				delete(alive, candidate)
			}
		}
		if len(alive) == 0 {
			break
		}
	}
	if !sawValue {
		return sheets.FieldTypeText
	}
	for _, candidate := range inferCandidates {
		if alive[candidate] {
			return candidate
		}
	}
	return sheets.FieldTypeText
}

// valueMatchesType checks one raw CSV value against a candidate type using
// the same sheets.CoerceValue rules the insert path enforces, so inference
// never picks a type the import would then fail to coerce.
func valueMatchesType(fieldType, value string) bool {
	if fieldType == sheets.FieldTypeCheckbox {
		switch strings.ToLower(value) {
		case "true", "false", "yes", "no":
			return true
		default:
			return false
		}
	}
	probe := &model.SheetField{ID: "fld_infer", Type: fieldType}
	_, err := sheets.CoerceValue(probe, value)
	return err == nil
}
