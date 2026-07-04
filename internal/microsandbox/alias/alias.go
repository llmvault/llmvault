// Package alias holds the shared alias-hostname rules used by BOTH the
// microsandbox control plane (claim/repoint, internal/microsandbox/control)
// and the apps service slug derivation (internal/apps). Keeping the ruleset in
// one leaf package guarantees a slug accepted at app_create is one that
// claimAlias will accept at deploy — the two used to drift, so a reserved or
// too-short app slug passed create and then hard-failed at publish.
package alias

import (
	"fmt"
	"regexp"
	"strings"
)

// Length bounds for an alias hostname stem.
const (
	MinLen = 3
	MaxLen = 63
)

var (
	charset    = regexp.MustCompile(`^[a-z0-9-]+$`)
	portPrefix = regexp.MustCompile(`^[0-9]{1,5}-`)
)

// Reserved protects operational hostnames and the preview scheme. The
// port-prefix pattern (e.g. "3000-...") is rejected separately because it
// collides with the {port}-{sandbox} preview host grammar.
var Reserved = map[string]bool{
	"www":      true,
	"api":      true,
	"admin":    true,
	"preview":  true,
	"static":   true,
	"app":      true,
	"apps":     true,
	"mail":     true,
	"gateway":  true,
	"health":   true,
	"metrics":  true,
	"internal": true,
	"runner":   true,
	"control":  true,
}

// Normalize lowercases and trims surrounding whitespace, matching how the
// control plane normalizes an inbound alias URL param before validating.
func Normalize(alias string) string {
	return strings.ToLower(strings.TrimSpace(alias))
}

// Validate reports whether alias is claimable: 3-63 chars, only lowercase
// letters/digits/hyphens, no leading/trailing hyphen, not shaped like a
// preview port prefix, and not reserved.
func Validate(alias string) error {
	if len(alias) < MinLen || len(alias) > MaxLen {
		return fmt.Errorf("alias must be between %d and %d characters", MinLen, MaxLen)
	}
	if !charset.MatchString(alias) {
		return fmt.Errorf("alias must contain only lowercase letters, digits, and hyphens")
	}
	if strings.HasPrefix(alias, "-") || strings.HasSuffix(alias, "-") {
		return fmt.Errorf("alias must not start or end with a hyphen")
	}
	if portPrefix.MatchString(alias) {
		return fmt.Errorf("alias must not look like a preview port prefix")
	}
	if Reserved[alias] {
		return fmt.Errorf("alias is reserved")
	}
	return nil
}
