package integrations

import (
	"context"
	"strings"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

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
