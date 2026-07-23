package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pb33f/libopenapi"
	highbase "github.com/pb33f/libopenapi/datamodel/high/base"
	v2high "github.com/pb33f/libopenapi/datamodel/high/v2"
)

// parseSpec parses a Swagger 2.0 spec and returns actions + referenced response schemas.
func parseSpec(specData []byte, cfg ServiceConfig) (*ParseResult, error) {
	doc, err := libopenapi.NewDocument(specData)
	if err != nil {
		return nil, fmt.Errorf("parsing Swagger document: %w", err)
	}

	model, errs := doc.BuildV2Model()
	if model == nil {
		return nil, fmt.Errorf("building v2 model: %v", errs)
	}

	basePath := ""
	if model.Model.BasePath != "" {
		basePath = strings.TrimRight(model.Model.BasePath, "/")
	}

	actions := make(map[string]ActionDef)
	schemas := make(map[string]FlatSchema)

	if model.Model.Paths == nil || model.Model.Paths.PathItems == nil {
		return &ParseResult{Actions: actions, Schemas: schemas}, nil
	}

	definitions := model.Model.Definitions
	definitionsMap := make(map[string]*highbase.SchemaProxy)
	if definitions != nil && definitions.Definitions != nil {
		for pair := definitions.Definitions.First(); pair != nil; pair = pair.Next() {
			definitionsMap[pair.Key()] = pair.Value()
		}
	}

	for pair := model.Model.Paths.PathItems.First(); pair != nil; pair = pair.Next() {
		rawPath := pair.Key()
		pathItem := pair.Value()

		fullPath := basePath + rawPath

		if !matchesFilters(fullPath, cfg.PathFilters, cfg.PathExcludes) {
			continue
		}

		ops := getV2Operations(pathItem)
		for method, op := range ops {
			selector, selected := selectedOperation(rawPath, method, cfg.OperationSelectors)
			if !selected {
				continue
			}
			if op.Deprecated && !selector.AllowDeprecated {
				continue
			}

			actionKey, displayName := deriveV2ActionKey(op, method, fullPath)
			override, hasOverride := cfg.ActionOverrides[op.OperationId]
			if hasOverride && override.Key != "" {
				actionKey = override.Key
			}
			if actionKey == "" {
				continue
			}

			if _, exists := actions[actionKey]; exists {
				continue
			}

			desc := ""
			if op.Description != "" {
				desc = truncateDescription(op.Description, 200)
			} else if op.Summary != "" {
				desc = truncateDescription(op.Summary, 200)
			}
			if hasOverride {
				if override.DisplayName != "" {
					displayName = override.DisplayName
				}
				if override.Description != "" {
					desc = override.Description
				}
			}

			resourceType := inferV2ResourceType(op, cfg.TagResourceMap)

			params, exec := buildV2ParamsAndExecution(op, pathItem.Parameters, method, fullPath, cfg.ExtraHeaders)
			if hasOverride && len(override.Parameters) > 0 {
				if !json.Valid(override.Parameters) {
					return nil, fmt.Errorf("operation %s has an invalid parameter override", op.OperationId)
				}
				params = append(json.RawMessage(nil), override.Parameters...)
			}

			access := inferAccess(method, actionKey, fullPath)
			if hasOverride && override.Access != "" {
				switch override.Access {
				case "read", "write":
					access = override.Access
				default:
					return nil, fmt.Errorf("operation %s has invalid access %q", op.OperationId, override.Access)
				}
			}

			responseSchemaRef := extractV2ResponseSchema(op, definitionsMap, schemas)

			actions[actionKey] = ActionDef{
				DisplayName:    displayName,
				Description:    desc,
				Access:         access,
				ResourceType:   resourceType,
				Parameters:     params,
				Execution:      exec,
				ResponseSchema: responseSchemaRef,
			}
		}
	}

	return &ParseResult{Actions: actions, Schemas: schemas}, nil
}

// getV2Operations returns method → operation for a Swagger 2.0 path item.
func getV2Operations(pi *v2high.PathItem) map[string]*v2high.Operation {
	ops := make(map[string]*v2high.Operation)
	if pi.Get != nil {
		ops["GET"] = pi.Get
	}
	if pi.Post != nil {
		ops["POST"] = pi.Post
	}
	if pi.Put != nil {
		ops["PUT"] = pi.Put
	}
	if pi.Patch != nil {
		ops["PATCH"] = pi.Patch
	}
	if pi.Delete != nil {
		ops["DELETE"] = pi.Delete
	}
	return ops
}
