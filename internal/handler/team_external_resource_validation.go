package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

// ExternalResourceRouteValidator verifies that a provider resource can be
// attached to a team route. Provider adapters own provider-specific checks
// such as joining a public Slack channel before it can receive conversations.
type ExternalResourceRouteValidator interface {
	ValidateExternalResourceRoute(context.Context, model.Connection, string, string) error
}

// WithExternalResourceRouteValidator adds the provider adapter used when team
// administrators create an external-resource route.
func WithExternalResourceRouteValidator(v ExternalResourceRouteValidator) TeamHandlerOption {
	return func(h *TeamHandler) {
		h.externalRouteValidator = v
	}
}

type externalResourceRouteValidationError struct {
	message string
}

func (e *externalResourceRouteValidationError) Error() string { return e.message }

func newExternalResourceRouteValidationError(format string, args ...any) error {
	return &externalResourceRouteValidationError{message: fmt.Sprintf(format, args...)}
}

func (h *TeamHandler) validateExternalResourceRoute(ctx context.Context, connection model.Connection, resourceType, resourceKey string) error {
	if h.externalRouteValidator == nil {
		return newExternalResourceRouteValidationError("external resource routing is not configured")
	}
	if strings.TrimSpace(resourceType) == "" || strings.TrimSpace(resourceKey) == "" {
		return newExternalResourceRouteValidationError("resource_type and resource_key are required")
	}
	return h.externalRouteValidator.ValidateExternalResourceRoute(ctx, connection, resourceType, resourceKey)
}
