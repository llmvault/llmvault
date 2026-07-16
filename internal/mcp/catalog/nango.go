package catalog

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
)

//go:embed nango-actions.json.gz
var nangoCatalogData []byte

// NangoAction is an action published in Nango's generated flows catalog.
// This metadata powers discovery only. Execution continues to use the explicit
// proxy configuration in ActionDef because self-hosted Nango does not deploy
// the generated cloud functions represented by this catalog.
type NangoAction struct {
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	InputSchema  json.RawMessage   `json:"input_schema,omitempty"`
	OutputSchema []json.RawMessage `json:"output_schema,omitempty"`
}

// NangoProvider is a provider and its generated action inventory.
type NangoProvider struct {
	ProviderConfigKey string        `json:"provider_config_key"`
	Actions           []NangoAction `json:"actions"`
}

type embeddedNangoCatalog struct {
	SchemaVersion  int             `json:"schema_version"`
	Source         string          `json:"source"`
	SourceRevision string          `json:"source_revision"`
	Providers      []NangoProvider `json:"providers"`
}

func (c *Catalog) parseNangoCatalog() {
	reader, err := gzip.NewReader(bytes.NewReader(nangoCatalogData))
	if err != nil {
		panic("catalog: failed to open embedded Nango catalog: " + err.Error())
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		panic("catalog: failed to read embedded Nango catalog: " + err.Error())
	}
	if err := reader.Close(); err != nil {
		panic("catalog: failed to close embedded Nango catalog: " + err.Error())
	}

	var embedded embeddedNangoCatalog
	if err := json.Unmarshal(data, &embedded); err != nil {
		panic("catalog: failed to parse embedded Nango catalog: " + err.Error())
	}
	if embedded.SchemaVersion != 1 {
		panic(fmt.Sprintf("catalog: unsupported Nango catalog schema version %d", embedded.SchemaVersion))
	}
	if embedded.Source == "" || embedded.SourceRevision == "" {
		panic("catalog: embedded Nango catalog is missing source metadata")
	}

	c.nangoSource = embedded.Source
	c.nangoSourceRevision = embedded.SourceRevision
	for i := range embedded.Providers {
		provider := &embedded.Providers[i]
		if provider.ProviderConfigKey == "" {
			panic("catalog: embedded Nango catalog contains an empty provider key")
		}
		if _, exists := c.nangoProviders[provider.ProviderConfigKey]; exists {
			panic("catalog: duplicate Nango provider " + provider.ProviderConfigKey)
		}
		seenActions := make(map[string]struct{}, len(provider.Actions))
		for _, action := range provider.Actions {
			if action.Name == "" {
				panic("catalog: Nango provider " + provider.ProviderConfigKey + " contains an empty action name")
			}
			if _, exists := seenActions[action.Name]; exists {
				panic("catalog: duplicate Nango action " + provider.ProviderConfigKey + "/" + action.Name)
			}
			seenActions[action.Name] = struct{}{}
		}
		c.nangoProviders[provider.ProviderConfigKey] = provider
	}
}

// NangoProvider returns the generated Nango action inventory for a provider.
func (c *Catalog) NangoProvider(providerConfigKey string) (*NangoProvider, bool) {
	provider, ok := c.nangoProviders[providerConfigKey]
	return provider, ok
}

// NangoSource identifies the exact generated catalog loaded into memory.
func (c *Catalog) NangoSource() (source string, revision string) {
	return c.nangoSource, c.nangoSourceRevision
}
