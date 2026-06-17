package hindsight

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

const (
	MemoryScopeProvider = "provider"
	MemoryScopeResource = "resource"
)

type MemoryTagInput struct {
	Scope        string `json:"scope"`
	Provider     string `json:"provider"`
	ResourceType string `json:"resource_type,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
	MemoryType   string `json:"memory_type,omitempty"`
}

type ValidatedMemoryTags struct {
	Input       MemoryTagInput
	RetainTags  []string
	FilterTags  []string
	Description string
}

type memoryTagValidationError struct {
	Field                string              `json:"field"`
	RejectedValue        string              `json:"rejected_value,omitempty"`
	Expected             string              `json:"expected"`
	AllowedProviders     []string            `json:"allowed_providers"`
	AllowedResourceTypes map[string][]string `json:"allowed_resource_types,omitempty"`
	AllowedResourceIDs   []string            `json:"allowed_resource_ids,omitempty"`
}

func (e memoryTagValidationError) Error() string {
	b, _ := json.Marshal(e)
	return "memory tag validation failed: " + string(b)
}

func ValidateRetainTags(ctx context.Context, db *gorm.DB, agent *model.Agent, input MemoryTagInput) (*ValidatedMemoryTags, error) {
	return validateMemoryTags(ctx, db, agent, input, true)
}

func ValidateRecallTags(ctx context.Context, db *gorm.DB, agent *model.Agent, input MemoryTagInput) (*ValidatedMemoryTags, error) {
	return validateMemoryTags(ctx, db, agent, input, false)
}

func validateMemoryTags(ctx context.Context, db *gorm.DB, agent *model.Agent, input MemoryTagInput, requireMemoryType bool) (*ValidatedMemoryTags, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("memory tag validation failed: agent org is required")
	}
	catalog, err := loadOrgMemoryTagCatalog(ctx, db, *agent.OrgID)
	if err != nil {
		return nil, err
	}
	input.Scope = normalizeTagValue(input.Scope)
	input.Provider = normalizeTagValue(input.Provider)
	input.ResourceType = normalizeTagValue(input.ResourceType)
	input.ResourceID = normalizeResourceID(input.Provider, input.ResourceType, input.ResourceID)
	input.MemoryType = normalizeTagValue(input.MemoryType)

	if input.Scope != MemoryScopeProvider && input.Scope != MemoryScopeResource {
		return nil, catalog.validationError("scope", input.Scope, `one of "provider" or "resource"`, input.Provider, input.ResourceType)
	}
	if input.Provider == "" {
		return nil, catalog.validationError("provider", "", "an active connected provider", "", "")
	}
	if !catalog.hasProvider(input.Provider) {
		return nil, catalog.validationError("provider", input.Provider, "one of the org's active connected providers", input.Provider, "")
	}
	if requireMemoryType {
		if input.MemoryType == "" {
			return nil, catalog.validationError("memory_type", "", "one of the supported memory_type values", input.Provider, input.ResourceType)
		}
		if !IsSupportedMemoryType(input.MemoryType) {
			return nil, catalog.validationError("memory_type", input.MemoryType, strings.Join(SupportedMemoryTypes, ", "), input.Provider, input.ResourceType)
		}
	} else if input.MemoryType != "" && !IsSupportedMemoryType(input.MemoryType) {
		return nil, catalog.validationError("memory_type", input.MemoryType, strings.Join(SupportedMemoryTypes, ", "), input.Provider, input.ResourceType)
	}
	if input.Scope == MemoryScopeResource {
		if input.ResourceType == "" {
			return nil, catalog.validationError("resource_type", "", "a known resource type for the selected provider", input.Provider, "")
		}
		if input.ResourceID == "" {
			return nil, catalog.validationError("resource_id", "", "a known resource id for the selected provider/resource_type", input.Provider, input.ResourceType)
		}
		if !catalog.hasResource(input.Provider, input.ResourceType, input.ResourceID) {
			return nil, catalog.validationError("resource_id", input.ResourceID, resourceExpectedFormat(input.Provider, input.ResourceType), input.Provider, input.ResourceType)
		}
	}

	filterTags := []string{"scope:" + input.Scope, "provider:" + input.Provider}
	if input.Scope == MemoryScopeResource {
		filterTags = append(filterTags,
			"resource_type:"+input.ResourceType,
			"resource:"+input.Provider+":"+input.ResourceType+":"+input.ResourceID,
		)
	}
	if input.MemoryType != "" {
		filterTags = append(filterTags, "memory_type:"+input.MemoryType)
	}
	retainTags := append([]string{}, filterTags...)
	retainTags = append(retainTags, "source:manual")
	return &ValidatedMemoryTags{
		Input:       input,
		RetainTags:  retainTags,
		FilterTags:  filterTags,
		Description: memoryTagDescription(input),
	}, nil
}

func MemoryTagGroups(tags []string) []any {
	if len(tags) == 0 {
		return nil
	}
	return []any{map[string]any{"tags": tags, "match": "all_strict"}}
}

func PreloadMemoryTagFilters(ctx context.Context, db *gorm.DB, agent *model.Agent) ([][]string, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, nil
	}
	catalog, err := loadOrgMemoryTagCatalog(ctx, db, *agent.OrgID)
	if err != nil {
		return nil, err
	}
	var filters [][]string
	for _, provider := range catalog.providers {
		filters = append(filters, []string{"scope:" + MemoryScopeProvider, "provider:" + provider})
	}
	if db == nil {
		return filters, nil
	}
	var current model.Agent
	if err := db.WithContext(ctx).Where("id = ?", agent.ID).First(&current).Error; err != nil {
		return filters, nil
	}
	connectionProviders, err := connectionProvidersForOrg(ctx, db, *agent.OrgID)
	if err != nil {
		return nil, err
	}
	for connectionID, raw := range current.Resources {
		provider := connectionProviders[connectionID]
		if provider == "" {
			continue
		}
		resources, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		for resourceType, rawItems := range resources {
			resourceType = normalizeTagValue(resourceType)
			for _, resourceID := range resourceIDsFromAny(provider, resourceType, rawItems) {
				if !catalog.hasResource(provider, resourceType, resourceID) {
					continue
				}
				filters = append(filters, []string{
					"scope:" + MemoryScopeResource,
					"provider:" + provider,
					"resource_type:" + resourceType,
					"resource:" + provider + ":" + resourceType + ":" + resourceID,
				})
			}
		}
	}
	return filters, nil
}
