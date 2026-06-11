package integrations

import "github.com/usehivy/hivy/internal/model"

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
