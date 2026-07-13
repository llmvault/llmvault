package integrations

// SupportedDefinition is the public, credential-free identity of an enabled
// platform integration manifest. Runtime configuration is deliberately loaded
// from the integrations table by the HTTP layer instead of exposed here.
type SupportedDefinition struct {
	ID          string
	Provider    string
	UniqueKey   string
	DisplayName string
}

// ListSupportedDefinitions returns every enabled integration Hivy ships with,
// including definitions that an operator has not configured in Nango yet.
func ListSupportedDefinitions(dir string) ([]SupportedDefinition, error) {
	manifests, err := loadManifests(dir)
	if err != nil {
		return nil, err
	}
	if err := validateManifests(manifests); err != nil {
		return nil, err
	}

	out := make([]SupportedDefinition, 0, len(manifests))
	for _, manifest := range manifests {
		if !enabled(manifest) {
			continue
		}
		out = append(out, SupportedDefinition{
			ID:          manifest.ID,
			Provider:    manifest.Provider,
			UniqueKey:   manifest.UniqueKey,
			DisplayName: manifest.DisplayName,
		})
	}
	return out, nil
}
