package mcpservers

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound              = errors.New("mcp servers: not found")
	ErrTeamNotFound          = errors.New("mcp servers: team not found")
	ErrAgentNotFound         = errors.New("mcp servers: agent not found")
	ErrAuthorizationNotFound = errors.New("mcp servers: authorization not found")
	ErrConflict              = errors.New("mcp servers: conflict")
	ErrOAuthStateInvalid     = errors.New("mcp servers: oauth state invalid or expired")
)

// ValidationError is safe to return to an API caller as a 422 response.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return "mcp servers: " + e.Message }

func validationErrorf(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}
