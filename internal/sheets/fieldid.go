// Package sheets implements org-scoped structured sheets: metadata tables plus
// JSONB row storage, a filter-AST → SQL query compiler, per-field-type value
// coercion, and an operation (undo) log.
package sheets

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
)

const (
	fieldIDPrefix  = "fld_"
	fieldIDRandLen = 10
	fieldIDCharset = "0123456789abcdefghijklmnopqrstuvwxyz"
)

// fieldIDPattern is the only shape a field ID may take. The query compiler
// re-validates IDs against this pattern before splicing them into SQL, so the
// generator and the pattern must stay in lockstep.
var fieldIDPattern = regexp.MustCompile(`^fld_[0-9a-z]{10}$`)

// NewFieldID returns a new field identifier of the form "fld_" followed by 10
// random base36 characters, sourced from crypto/rand.
func NewFieldID() (string, error) {
	max := big.NewInt(int64(len(fieldIDCharset)))
	buf := make([]byte, fieldIDRandLen)
	for i := range buf {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate field id: %w", err)
		}
		buf[i] = fieldIDCharset[n.Int64()]
	}
	return fieldIDPrefix + string(buf), nil
}

// ValidFieldID reports whether id matches the canonical field ID shape.
func ValidFieldID(id string) bool {
	return fieldIDPattern.MatchString(id)
}
