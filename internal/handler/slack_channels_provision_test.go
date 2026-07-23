package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestSlackResourceRouteValidatorRejectsUnsupportedRouteScope(t *testing.T) {
	validator := &SlackResourceRouteValidator{}
	tests := []struct {
		name         string
		provider     string
		resourceType string
		wantError    string
	}{
		{
			name:         "non-Slack connection",
			provider:     "github-app",
			resourceType: "slack_channel",
			wantError:    "supports Slack connections only",
		},
		{
			name:         "non-channel Slack resource",
			provider:     "slack",
			resourceType: "slack_user",
			wantError:    "resource_type is not supported for Slack",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validator.ValidateExternalResourceRoute(
				context.Background(),
				model.Connection{Integration: model.Integration{Provider: test.provider}},
				test.resourceType,
				"resource-123",
			)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v, want message containing %q", err, test.wantError)
			}
		})
	}
}
