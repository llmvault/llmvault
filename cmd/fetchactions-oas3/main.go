// cmd/fetchactions-oas3 generates per-provider action files from OpenAPI 3.x specs.
//
// Usage:
//
//	go run ./cmd/fetchactions-oas3                     # generate all OAS3 providers
//	go run ./cmd/fetchactions-oas3 -provider github    # generate one service
//	go run ./cmd/fetchactions-oas3 -force              # bypass spec cache
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	provider := flag.String("provider", "", "Generate actions for a single service (by name)")
	force := flag.Bool("force", false, "Force re-download of specs (bypass cache)")
	flag.Parse()

	if err := run(*provider, *force); err != nil {
		fmt.Fprintf(os.Stderr, "fetchactions-oas3: %v\n", err)
		os.Exit(1)
	}
}

func run(providerFilter string, force bool) error {
	metadata, err := loadMetadata()
	if err != nil {
		return err
	}

	services := AllServices()
	if providerFilter != "" {
		var filtered []ServiceConfig
		for _, svc := range services {
			if svc.Name == providerFilter {
				filtered = append(filtered, svc)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("unknown provider %q", providerFilter)
		}
		services = filtered
	}

	totalFiles := 0
	totalActions := 0

	for _, svc := range services {
		sources := append([]string{svc.SpecSource}, svc.AdditionalSpecSources...)
		result := &ParseResult{Actions: make(map[string]ActionDef), Schemas: make(map[string]FlatSchema)}
		failed := false
		for index, source := range sources {
			fmt.Printf("[%s] Fetching spec %d/%d...\n", svc.Name, index+1, len(sources))
			specData, err := fetchSpec(source, force)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] ERROR: %v (skipping)\n", svc.Name, err)
				failed = true
				break
			}
			fmt.Printf("[%s] Spec downloaded (%d KB)\n", svc.Name, len(specData)/1024)

			fmt.Printf("[%s] Parsing operations...\n", svc.Name)
			parsed, err := parseSpec(specData, svc)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] ERROR parsing: %v (skipping)\n", svc.Name, err)
				failed = true
				break
			}
			if len(sources) > 1 {
				namespaceParseResultSchemas(parsed, fmt.Sprintf("%s_%d", svc.Name, index+1))
			}
			if err := mergeParseResult(result, parsed); err != nil {
				fmt.Fprintf(os.Stderr, "[%s] ERROR merging specs: %v (skipping)\n", svc.Name, err)
				failed = true
				break
			}
		}
		if failed {
			continue
		}

		if len(result.Actions) == 0 {
			fmt.Fprintf(os.Stderr, "[%s] WARNING: no actions generated (skipping)\n", svc.Name)
			continue
		}

		// Validate actions.
		errs := validateActions(result.Actions)
		if len(errs) > 0 {
			for _, e := range errs {
				fmt.Fprintf(os.Stderr, "[%s] VALIDATION: %s\n", svc.Name, e)
			}
		}

		if err := writeProviderFiles(svc, result, metadata); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] ERROR writing: %v\n", svc.Name, err)
			continue
		}

		resourceScoped := 0
		for _, action := range result.Actions {
			if action.ResourceType != "" {
				resourceScoped++
			}
		}

		for _, id := range svc.NangoProviders {
			fmt.Printf("  %s.actions.json: %d actions, %d schemas (%d resource-scoped)\n", id, len(result.Actions), len(result.Schemas), resourceScoped)
		}
		totalFiles += len(svc.NangoProviders)
		totalActions += len(result.Actions)
	}

	fmt.Printf("\nTotal: %d files, %d unique actions\n", totalFiles, totalActions)
	return nil
}

// namespaceParseResultSchemas prevents equal component names from separate
// OpenAPI documents from overwriting one another in the generated provider file.
func namespaceParseResultSchemas(result *ParseResult, namespace string) {
	if namespace == "" || len(result.Schemas) == 0 {
		return
	}

	refs := make(map[string]string, len(result.Schemas))
	for name := range result.Schemas {
		refs[name] = namespace + "_" + name
	}
	resolveRef := func(ref string) string {
		if replacement, ok := refs[ref]; ok {
			return replacement
		}
		return ref
	}

	for actionKey, action := range result.Actions {
		action.ResponseSchema = resolveRef(action.ResponseSchema)
		result.Actions[actionKey] = action
	}

	namespaced := make(map[string]FlatSchema, len(result.Schemas))
	for name, schema := range result.Schemas {
		for propertyName, property := range schema.Properties {
			property.SchemaRef = resolveRef(property.SchemaRef)
			schema.Properties[propertyName] = property
		}
		if schema.Items != nil {
			schema.Items.Ref = resolveRef(schema.Items.Ref)
		}
		namespaced[resolveRef(name)] = schema
	}
	result.Schemas = namespaced
}

// mergeParseResult combines operations parsed from multiple source documents.
func mergeParseResult(target, source *ParseResult) error {
	for key, action := range source.Actions {
		if existing, exists := target.Actions[key]; exists {
			existingJSON, _ := json.Marshal(existing)
			actionJSON, _ := json.Marshal(action)
			if string(existingJSON) != string(actionJSON) {
				return fmt.Errorf("conflicting action %q", key)
			}
			continue
		}
		target.Actions[key] = action
	}
	for key, schema := range source.Schemas {
		if existing, exists := target.Schemas[key]; exists {
			existingJSON, _ := json.Marshal(existing)
			schemaJSON, _ := json.Marshal(schema)
			if string(existingJSON) != string(schemaJSON) {
				return fmt.Errorf("conflicting schema %q", key)
			}
			continue
		}
		target.Schemas[key] = schema
	}
	return nil
}

// validateActions checks generated actions for common issues.
func validateActions(actions map[string]ActionDef) []string {
	var errs []string
	for key, a := range actions {
		if a.DisplayName == "" {
			errs = append(errs, fmt.Sprintf("%s: empty display_name", key))
		}
		if a.Execution == nil {
			errs = append(errs, fmt.Sprintf("%s: missing execution config", key))
			continue
		}
		if a.Execution.Method == "" {
			errs = append(errs, fmt.Sprintf("%s: empty execution.method", key))
		}
		if a.Execution.Path == "" {
			errs = append(errs, fmt.Sprintf("%s: empty execution.path", key))
		}
	}
	return errs
}
