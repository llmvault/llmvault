package handler

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

func boolPtr(v bool) *bool { return &v }
func intPtr(v int) *int    { return &v }

// trimmedRef returns nil for empty/whitespace inputs so we don't store empty
// strings; some model columns are nullable.
func trimmedRef(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}

func derefBool(p *bool, fallback bool) bool {
	if p != nil {
		return *p
	}
	return fallback
}

func providerRequiresWebhookConfig(provider string) bool {
	cat := catalog.Global()
	pt, ok := cat.GetProviderTriggers(provider)
	if !ok {
		pt, ok = cat.GetProviderTriggersForVariant(provider)
	}
	if !ok || pt.WebhookConfig == nil {
		return false
	}
	return pt.WebhookConfig.WebhookURLRequired
}

func safeConnectionMeta(meta model.JSON) model.JSON {
	if meta == nil {
		return nil
	}
	safe := model.JSON{}
	for key, value := range meta {
		if key == "credentials" {
			continue
		}
		safe[key] = value
	}
	return safe
}

func isDuplicateKeyError(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey) ||
		(err != nil && strings.Contains(err.Error(), "duplicate key"))
}

func buildNangoConfig(integResp map[string]any, template map[string]any, callbackURL string) model.JSON {
	return model.JSON(nango.BuildConfig(integResp, template, callbackURL))
}

func validateCredentials(provider nango.Provider, creds *nango.Credentials) error {
	mode := provider.AuthMode
	switch mode {
	case "OAUTH1", "OAUTH2", "TBA":
		if creds == nil {
			return fmt.Errorf("credentials required for %s auth mode", mode)
		}
		if creds.Type != mode {
			return fmt.Errorf("credentials.type must be %q for provider %q", mode, provider.Name)
		}
		if creds.ClientID == "" {
			return fmt.Errorf("client_id is required for %s auth mode", mode)
		}
		if creds.ClientSecret == "" {
			return fmt.Errorf("client_secret is required for %s auth mode", mode)
		}
	case "APP":
		if creds == nil {
			return fmt.Errorf("credentials required for APP auth mode")
		}
		if creds.Type != "APP" {
			return fmt.Errorf("credentials.type must be \"APP\" for provider %q", provider.Name)
		}
	}
	return nil
}
