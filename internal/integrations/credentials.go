package integrations

import (
	"fmt"

	"github.com/usehivy/hivy/internal/nango"
)

func credentialsRequired(mode string) bool {
	switch mode {
	case "OAUTH1", "OAUTH2", "TBA", "APP", "CUSTOM", "MCP_OAUTH2", "MCP_OAUTH2_GENERIC", "INSTALL_PLUGIN":
		return true
	default:
		return false
	}
}

func validateCredentials(provider nango.Provider, creds *nango.Credentials) error {
	if creds == nil {
		return nil
	}
	mode := provider.AuthMode
	switch mode {
	case "OAUTH1", "OAUTH2", "TBA":
		if creds.Type != mode {
			return fmt.Errorf("credentials.type must be %q for provider %q", mode, provider.Name)
		}
		if creds.ClientID == "" || creds.ClientSecret == "" {
			return fmt.Errorf("client_id and client_secret are required for %s auth mode", mode)
		}
	case "APP":
		if creds.Type != "APP" {
			return fmt.Errorf("credentials.type must be \"APP\" for provider %q", provider.Name)
		}
		if creds.AppID == "" || creds.PrivateKey == "" {
			return fmt.Errorf("app_id and private_key are required for APP auth mode")
		}
		if provider.Name == "github-app" && creds.AppLink == "" {
			return fmt.Errorf("app_link is required for provider %q", provider.Name)
		}
	case "CUSTOM":
		if provider.Name == "github-app-oauth" {
			if creds.Type != "APP" {
				return fmt.Errorf("credentials.type must be \"APP\" for provider %q", provider.Name)
			}
			if creds.AppID == "" || creds.AppLink == "" || creds.PrivateKey == "" {
				return fmt.Errorf("app_id, app_link, and private_key are required for provider %q", provider.Name)
			}
		}
	}
	return nil
}
