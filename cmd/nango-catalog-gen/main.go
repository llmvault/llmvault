// Command nango-catalog-gen converts Nango's generated flows catalog into the
// compact artifact embedded by the Hivy backend. The source file is generated
// by the usehivy/integrations fork at packages/shared/flows.zero.json.
package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
)

type sourceProvider struct {
	ProviderConfigKey string         `json:"providerConfigKey"`
	Actions           []sourceAction `json:"actions"`
}

type sourceAction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Input       string `json:"input"`
	Output      any    `json:"output"`
	JSONSchema  struct {
		Definitions map[string]json.RawMessage `json:"definitions"`
	} `json:"json_schema"`
}

type generatedCatalog struct {
	SchemaVersion  int                 `json:"schema_version"`
	Source         string              `json:"source"`
	SourceRevision string              `json:"source_revision"`
	Providers      []generatedProvider `json:"providers"`
}

type generatedProvider struct {
	ProviderConfigKey string            `json:"provider_config_key"`
	Actions           []generatedAction `json:"actions"`
}

type generatedAction struct {
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	InputSchema  json.RawMessage   `json:"input_schema,omitempty"`
	OutputSchema []json.RawMessage `json:"output_schema,omitempty"`
}

func main() {
	sourcePath := flag.String("source", "", "path to packages/shared/flows.zero.json")
	outputPath := flag.String("output", "internal/mcp/catalog/nango-actions.json.gz", "generated artifact path")
	revision := flag.String("revision", "", "source integrations git revision")
	flag.Parse()

	if *sourcePath == "" || *revision == "" {
		fmt.Fprintln(os.Stderr, "-source and -revision are required")
		os.Exit(2)
	}

	data, err := os.ReadFile(*sourcePath)
	if err != nil {
		fatal(err)
	}

	var source []sourceProvider
	if err := json.Unmarshal(data, &source); err != nil {
		fatal(err)
	}

	result := generatedCatalog{
		SchemaVersion:  1,
		Source:         "usehivy/integrations:packages/shared/flows.zero.json",
		SourceRevision: *revision,
		Providers:      make([]generatedProvider, 0, len(source)),
	}
	for _, provider := range source {
		generated := generatedProvider{
			ProviderConfigKey: provider.ProviderConfigKey,
			Actions:           make([]generatedAction, 0, len(provider.Actions)),
		}
		for _, action := range provider.Actions {
			item := generatedAction{
				Name:        action.Name,
				Description: action.Description,
			}
			if action.Input != "" {
				item.InputSchema = action.JSONSchema.Definitions[action.Input]
			}
			for _, outputName := range outputNames(action.Output) {
				if schema := action.JSONSchema.Definitions[outputName]; len(schema) > 0 {
					item.OutputSchema = append(item.OutputSchema, schema)
				}
			}
			generated.Actions = append(generated.Actions, item)
		}
		sort.Slice(generated.Actions, func(i, j int) bool {
			return generated.Actions[i].Name < generated.Actions[j].Name
		})
		result.Providers = append(result.Providers, generated)
	}
	sort.Slice(result.Providers, func(i, j int) bool {
		return result.Providers[i].ProviderConfigKey < result.Providers[j].ProviderConfigKey
	})

	output, err := os.Create(*outputPath)
	if err != nil {
		fatal(err)
	}
	gz := gzip.NewWriter(output)
	encoder := json.NewEncoder(gz)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		_ = output.Close()
		fatal(err)
	}
	if err := gz.Close(); err != nil {
		_ = output.Close()
		fatal(err)
	}
	if err := output.Close(); err != nil {
		fatal(err)
	}
}

func outputNames(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if name, ok := item.(string); ok {
				result = append(result, name)
			}
		}
		return result
	default:
		return nil
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
