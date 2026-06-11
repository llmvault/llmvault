package integrations

import (
	"context"
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

type AdminDefinition struct {
	ID                string                    `json:"id"`
	Provider          string                    `json:"provider"`
	NangoProvider     string                    `json:"nango_provider"`
	UniqueKey         string                    `json:"unique_key"`
	DisplayName       string                    `json:"display_name"`
	Enabled           bool                      `json:"enabled"`
	Required          bool                      `json:"required"`
	SupportsRAGSource bool                      `json:"supports_rag_source"`
	AuthMode          string                    `json:"auth_mode"`
	CredentialFields  []AdminCredentialField    `json:"credential_fields"`
	FixedCredentials  []AdminFixedCredential    `json:"fixed_credentials,omitempty"`
	Meta              model.JSON                `json:"meta,omitempty"`
	Existing          *AdminExistingIntegration `json:"existing,omitempty"`
}

type AdminCredentialField struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Required    bool   `json:"required"`
	Secret      bool   `json:"secret"`
	Multiline   bool   `json:"multiline"`
	Placeholder string `json:"placeholder,omitempty"`
}

type AdminFixedCredential struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Value string `json:"value"`
}

type AdminExistingIntegration struct {
	ID                string `json:"id"`
	UniqueKey         string `json:"unique_key"`
	DisplayName       string `json:"display_name"`
	Managed           bool   `json:"managed"`
	ActiveConnections int64  `json:"active_connections"`
	UpdatedAt         string `json:"updated_at"`
}

func (s *Seeder) ListAdminDefinitions(ctx context.Context, dir string) ([]AdminDefinition, error) {
	manifests, err := loadManifests(dir)
	if err != nil {
		return nil, err
	}
	if err := validateManifests(manifests); err != nil {
		return nil, err
	}

	out := make([]AdminDefinition, 0, len(manifests))
	for _, manifest := range manifests {
		def := s.adminDefinition(ctx, manifest)
		out = append(out, def)
	}
	return out, nil
}

func (s *Seeder) adminDefinition(ctx context.Context, m Manifest) AdminDefinition {
	providerName := nangoProvider(m)
	provider, _ := s.nango.GetProvider(providerName)
	def := AdminDefinition{
		ID:                m.ID,
		Provider:          m.Provider,
		NangoProvider:     providerName,
		UniqueKey:         m.UniqueKey,
		DisplayName:       m.DisplayName,
		Enabled:           enabled(m),
		Required:          m.Required,
		SupportsRAGSource: m.SupportsRAGSource,
		AuthMode:          provider.AuthMode,
		CredentialFields:  adminCredentialFields(m, provider),
		FixedCredentials:  adminFixedCredentials(m),
		Meta:              m.Meta,
	}

	var existing model.Integration
	err := s.db.WithContext(ctx).
		Where("deleted_at IS NULL").
		Where("(managed_by = ? AND managed_id = ?) OR unique_key = ?", managedBy, m.ID, m.UniqueKey).
		First(&existing).Error
	if err == nil {
		active, _ := s.activeConnectionCount(ctx, existing.ID)
		def.Existing = &AdminExistingIntegration{
			ID:                existing.ID.String(),
			UniqueKey:         existing.UniqueKey,
			DisplayName:       existing.DisplayName,
			Managed:           existing.ManagedBy == managedBy,
			ActiveConnections: active,
			UpdatedAt:         existing.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	return def
}

func adminCredentialFields(m Manifest, provider nango.Provider) []AdminCredentialField {
	if m.Credentials == nil {
		return nil
	}

	c := m.Credentials
	fields := make([]AdminCredentialField, 0, 6)
	add := func(include bool, name, label string, secret, multiline bool, placeholder string) {
		if !include {
			return
		}
		fields = append(fields, AdminCredentialField{
			Name:        name,
			Label:       label,
			Required:    true,
			Secret:      secret,
			Multiline:   multiline,
			Placeholder: placeholder,
		})
	}

	add(c.ClientID, "client_id", "Client ID", false, false, "")
	add(c.ClientSecret, "client_secret", "Client secret", true, false, "")
	add(c.AppID, "app_id", "App ID", false, false, "")
	add(c.AppLinkField, "app_link", "App link", false, false, "https://github.com/apps/your-app")
	add(c.PrivateKey, "private_key", "Private key", true, true, "-----BEGIN PRIVATE KEY-----")
	add(c.WebhookSecret, "webhook_secret", "Webhook secret", true, false, "")
	add(c.Username, "username", "Username", false, false, "")
	add(c.Password, "password", "Password", true, false, "")

	if len(fields) == 0 && credentialsRequired(provider.AuthMode) {
		switch provider.AuthMode {
		case "OAUTH1", "OAUTH2", "TBA":
			fields = append(fields,
				AdminCredentialField{Name: "client_id", Label: "Client ID", Required: true},
				AdminCredentialField{Name: "client_secret", Label: "Client secret", Required: true, Secret: true},
			)
		case "APP":
			fields = append(fields,
				AdminCredentialField{Name: "app_id", Label: "App ID", Required: true},
				AdminCredentialField{Name: "private_key", Label: "Private key", Required: true, Secret: true, Multiline: true},
			)
		}
	}

	return fields
}

func adminFixedCredentials(m Manifest) []AdminFixedCredential {
	if m.Credentials == nil {
		return nil
	}
	c := m.Credentials
	fixed := make([]AdminFixedCredential, 0, 3)
	add := func(name, label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		fixed = append(fixed, AdminFixedCredential{Name: name, Label: label, Value: value})
	}
	add("type", "Auth type", c.Type)
	add("scopes", "Scopes", c.Scopes)
	add("app_link", "App link", c.AppLink)
	return fixed
}

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
