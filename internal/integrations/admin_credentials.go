package integrations

import (
	"context"
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/nango"
)

func (s *Seeder) UpsertAdmin(ctx context.Context, id string, creds *nango.Credentials) (string, AdminDefinition, error) {
	manifests, err := loadManifests("global/integrations")
	if err != nil {
		return "", AdminDefinition{}, err
	}
	if err := validateManifests(manifests); err != nil {
		return "", AdminDefinition{}, err
	}

	for _, manifest := range manifests {
		if manifest.ID != id {
			continue
		}
		state, err := s.syncOneWithCredentials(ctx, manifest, creds)
		return state, s.adminDefinition(ctx, manifest), err
	}
	return "", AdminDefinition{}, fmt.Errorf("integration definition %q not found", id)
}

func (s *Seeder) syncOneWithCredentials(ctx context.Context, m Manifest, provided *nango.Credentials) (string, error) {
	if !enabled(m) {
		return s.disableOne(ctx, m)
	}
	if !m.AllowNoCatalog {
		if _, ok := s.catalog.GetProvider(m.Provider); !ok {
			return "", fmt.Errorf("%s: provider %q has no MCP catalog entry", m.SourcePath, m.Provider)
		}
	}
	provider, ok := s.nango.GetProvider(nangoProvider(m))
	if !ok {
		return "", fmt.Errorf("%s: Nango provider %q not found", m.SourcePath, nangoProvider(m))
	}
	creds, err := credentialsFromAdmin(m, provider, provided)
	if err != nil {
		return "", err
	}
	hash, err := manifestHash(m)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", m.SourcePath, err)
	}
	if err := s.upsertNango(ctx, m, creds); err != nil {
		return "", err
	}
	cfg, err := s.fetchConfig(ctx, m)
	if err != nil {
		return "", err
	}
	return s.upsertDB(ctx, m, cfg, hash)
}

func credentialsFromAdmin(m Manifest, provider nango.Provider, provided *nango.Credentials) (*nango.Credentials, error) {
	if m.Credentials == nil {
		if credentialsRequired(provider.AuthMode) && provided == nil {
			return nil, fmt.Errorf("credentials required for %s auth mode", provider.AuthMode)
		}
		if provided == nil {
			return nil, nil
		}
		if provided.Type == "" {
			provided.Type = provider.AuthMode
		}
		if err := validateCredentials(provider, provided); err != nil {
			return nil, err
		}
		return provided, nil
	}

	c := m.Credentials
	creds := &nango.Credentials{
		Type:          firstNonEmpty(providedString(provided, "type"), strings.TrimSpace(c.Type), provider.AuthMode),
		Scopes:        firstNonEmpty(providedString(provided, "scopes"), strings.TrimSpace(c.Scopes)),
		AppLink:       firstNonEmpty(providedString(provided, "app_link"), strings.TrimSpace(c.AppLink)),
		ClientName:    firstNonEmpty(providedString(provided, "client_name"), strings.TrimSpace(c.ClientName)),
		ClientUri:     firstNonEmpty(providedString(provided, "client_uri"), strings.TrimSpace(c.ClientURI)),
		ClientLogoUri: firstNonEmpty(providedString(provided, "client_logo_uri"), strings.TrimSpace(c.ClientLogoURI)),
	}
	if provided != nil {
		creds.ClientID = strings.TrimSpace(provided.ClientID)
		creds.ClientSecret = strings.TrimSpace(provided.ClientSecret)
		creds.AppID = strings.TrimSpace(provided.AppID)
		if provided.AppLink != "" {
			creds.AppLink = strings.TrimSpace(provided.AppLink)
		}
		creds.PrivateKey = strings.TrimSpace(provided.PrivateKey)
		creds.WebhookSecret = strings.TrimSpace(provided.WebhookSecret)
		creds.Username = strings.TrimSpace(provided.Username)
		creds.Password = strings.TrimSpace(provided.Password)
	}

	missing := missingAdminCredentials(m, creds)
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing credential field(s): %s", strings.Join(missing, ", "))
	}
	if err := validateCredentials(provider, creds); err != nil {
		return nil, err
	}
	return creds, nil
}

func missingAdminCredentials(m Manifest, creds *nango.Credentials) []string {
	if m.Credentials == nil {
		return nil
	}
	c := m.Credentials
	missing := make([]string, 0, 4)
	check := func(required bool, value, label string) {
		if required && strings.TrimSpace(value) == "" {
			missing = append(missing, label)
		}
	}
	check(c.ClientID, creds.ClientID, "client_id")
	check(c.ClientSecret, creds.ClientSecret, "client_secret")
	check(c.AppID, creds.AppID, "app_id")
	check(c.AppLinkField, creds.AppLink, "app_link")
	check(c.PrivateKey, creds.PrivateKey, "private_key")
	check(c.WebhookSecret, creds.WebhookSecret, "webhook_secret")
	check(c.Username, creds.Username, "username")
	check(c.Password, creds.Password, "password")
	return missing
}

func providedString(creds *nango.Credentials, name string) string {
	if creds == nil {
		return ""
	}
	switch name {
	case "type":
		return creds.Type
	case "scopes":
		return creds.Scopes
	case "app_link":
		return creds.AppLink
	case "client_name":
		return creds.ClientName
	case "client_uri":
		return creds.ClientUri
	case "client_logo_uri":
		return creds.ClientLogoUri
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
