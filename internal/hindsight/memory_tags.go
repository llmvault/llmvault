package hindsight

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/connectionaccess"
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

type memoryTagCatalog struct {
	providers []string
	resources map[string]map[string]map[string]struct{}
}

func loadOrgMemoryTagCatalog(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (*memoryTagCatalog, error) {
	catalog := &memoryTagCatalog{resources: map[string]map[string]map[string]struct{}{}}
	if db == nil || orgID == uuid.Nil {
		return catalog, nil
	}
	var connections []model.Connection
	if err := db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Where("connections.org_id = ? AND connections.revoked_at IS NULL", orgID).
		Order("integrations.provider ASC, connections.created_at ASC").
		Find(&connections).Error; err != nil {
		return nil, fmt.Errorf("load memory tag providers: %w", err)
	}
	providerSeen := map[string]struct{}{}
	for _, conn := range connections {
		provider := normalizeTagValue(conn.Integration.Provider)
		if provider == "" {
			continue
		}
		providerSeen[provider] = struct{}{}
		catalog.addResources(provider, connectionaccess.EffectiveResources(model.JSON{}, conn))
	}

	var agents []model.Agent
	if err := db.WithContext(ctx).
		Where("org_id = ? AND status <> ?", orgID, "archived").
		Find(&agents).Error; err != nil {
		return nil, fmt.Errorf("load memory tag agent resources: %w", err)
	}
	providerByConnection := map[string]string{}
	for _, conn := range connections {
		providerByConnection[conn.ID.String()] = normalizeTagValue(conn.Integration.Provider)
	}
	for _, agent := range agents {
		for connectionID, raw := range agent.Resources {
			provider := providerByConnection[connectionID]
			if provider == "" {
				continue
			}
			resources, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			catalog.addResources(provider, model.JSON(resources))
		}
	}
	for provider := range providerSeen {
		catalog.providers = append(catalog.providers, provider)
	}
	sort.Strings(catalog.providers)
	return catalog, nil
}

func connectionProvidersForOrg(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (map[string]string, error) {
	var connections []model.Connection
	if err := db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Where("connections.org_id = ? AND connections.revoked_at IS NULL", orgID).
		Find(&connections).Error; err != nil {
		return nil, fmt.Errorf("load memory tag connection providers: %w", err)
	}
	out := map[string]string{}
	for _, conn := range connections {
		out[conn.ID.String()] = normalizeTagValue(conn.Integration.Provider)
	}
	return out, nil
}

func (c *memoryTagCatalog) addResources(provider string, resources model.JSON) {
	if c.resources[provider] == nil {
		c.resources[provider] = map[string]map[string]struct{}{}
	}
	for resourceType, rawItems := range resources {
		resourceType = normalizeTagValue(resourceType)
		if resourceType == "" {
			continue
		}
		if c.resources[provider][resourceType] == nil {
			c.resources[provider][resourceType] = map[string]struct{}{}
		}
		for _, id := range resourceIDsFromAny(provider, resourceType, rawItems) {
			c.resources[provider][resourceType][id] = struct{}{}
		}
	}
}

func resourceIDsFromAny(provider, resourceType string, raw any) []string {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := stringAny(m["id"])
		if connectionaccess.IsGitHubProvider(provider) && resourceType == "repository" {
			if fullName := stringAny(m["full_name"]); fullName != "" {
				id = fullName
			}
		}
		id = normalizeResourceID(provider, resourceType, id)
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

func (c *memoryTagCatalog) hasProvider(provider string) bool {
	for _, allowed := range c.providers {
		if allowed == provider {
			return true
		}
	}
	return false
}

func (c *memoryTagCatalog) hasResource(provider, resourceType, resourceID string) bool {
	types := c.resources[provider]
	if types == nil {
		return false
	}
	ids := types[resourceType]
	if ids == nil {
		return false
	}
	_, ok := ids[resourceID]
	return ok
}

func (c *memoryTagCatalog) validationError(field, rejected, expected, provider, resourceType string) error {
	return memoryTagValidationError{
		Field:                field,
		RejectedValue:        rejected,
		Expected:             expected,
		AllowedProviders:     append([]string{}, c.providers...),
		AllowedResourceTypes: c.allowedResourceTypes(),
		AllowedResourceIDs:   c.allowedResourceIDs(provider, resourceType),
	}
}

func (c *memoryTagCatalog) allowedResourceTypes() map[string][]string {
	out := map[string][]string{}
	for provider, byType := range c.resources {
		for resourceType := range byType {
			out[provider] = append(out[provider], resourceType)
		}
		sort.Strings(out[provider])
	}
	return out
}

func (c *memoryTagCatalog) allowedResourceIDs(provider, resourceType string) []string {
	if provider == "" || resourceType == "" {
		return nil
	}
	ids := c.resources[provider][resourceType]
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func normalizeTagValue(value string) string {
	return sanitizeMemoryTagValue(value)
}

func normalizeResourceID(provider, resourceType, value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if connectionaccess.IsGitHubProvider(provider) && resourceType == "repository" {
		parts := strings.Split(value, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return value
		}
		return parts[0] + "/" + parts[1]
	}
	return sanitizeMemoryTagValue(value)
}

func stringAny(value any) string {
	s, _ := value.(string)
	return strings.TrimSpace(s)
}

func resourceExpectedFormat(provider, resourceType string) string {
	if connectionaccess.IsGitHubProvider(provider) && resourceType == "repository" {
		return `one of the connected GitHub repositories in "owner/repo" format`
	}
	return "one of the known resource IDs for the selected provider/resource_type"
}

func memoryTagDescription(input MemoryTagInput) string {
	if input.Scope == MemoryScopeResource {
		return fmt.Sprintf("%s %s %s", input.Provider, input.ResourceType, input.ResourceID)
	}
	return input.Provider + " provider"
}
