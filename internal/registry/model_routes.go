package registry

import (
	"fmt"
	"sort"
)

type ModelRoute struct {
	ProviderID string
	ModelID    string
	// CanonicalModelID is set when this route intentionally degrades to a
	// different catalog model. Empty means the originally requested model.
	CanonicalModelID string
}

type RoutedModel struct {
	Model
	ProviderIDs []string
}

type ResolvedModelRoute struct {
	CanonicalID string
	ProviderID  string
	UpstreamID  string
	Model       Model
}

func (r *Registry) ResolveModel(providerID, canonicalID string) (ResolvedModelRoute, bool) {
	hivyModel, ok := hivyModelsByID[canonicalID]
	if !ok {
		return ResolvedModelRoute{}, false
	}
	for _, route := range hivyModel.Routes {
		if route.ProviderID != providerID {
			continue
		}
		mdl, ok := r.providerModel(route.ProviderID, route.ModelID)
		if !ok {
			return ResolvedModelRoute{}, false
		}
		mdl.ID = canonicalID
		mdl.Hidden = false
		mdl = r.modelWithNewWindow(mdl, hivyModel)
		return ResolvedModelRoute{
			CanonicalID: canonicalID,
			ProviderID:  route.ProviderID,
			UpstreamID:  route.ModelID,
			Model:       mdl,
		}, true
	}
	return ResolvedModelRoute{}, false
}

func (r *Registry) CanonicalModel(canonicalID string) (RoutedModel, bool) {
	hivyModel, ok := hivyModelsByID[canonicalID]
	if !ok {
		return RoutedModel{}, false
	}
	for _, route := range hivyModel.Routes {
		mdl, ok := r.providerModel(route.ProviderID, route.ModelID)
		if !ok {
			continue
		}
		mdl.ID = canonicalID
		mdl.Hidden = false
		mdl = r.modelWithNewWindow(mdl, hivyModel)
		return RoutedModel{Model: mdl, ProviderIDs: providerIDsForRoutes(hivyModel.Routes)}, true
	}
	return RoutedModel{}, false
}

func (r *Registry) ProviderPreferenceForModel(canonicalID string) []string {
	routes := r.ProxyRoutesForModel(canonicalID)
	providerIDs := make([]string, 0, len(routes))
	for _, route := range routes {
		if _, ok := r.providerModel(route.ProviderID, route.ModelID); !ok {
			continue
		}
		providerIDs = appendIfMissing(providerIDs, route.ProviderID)
	}
	return providerIDs
}

// ProxyRoutesForModel returns the ordered routes used by the OpenAI-compatible
// proxy. Explicit ProxyRoutes take precedence. Legacy models without an
// explicit chain retain their catalog route order.
func (r *Registry) ProxyRoutesForModel(canonicalID string) []ModelRoute {
	hivyModel, ok := hivyModelsByID[canonicalID]
	if !ok {
		return nil
	}
	if len(hivyModel.ProxyRoutes) > 0 {
		return append([]ModelRoute(nil), hivyModel.ProxyRoutes...)
	}

	return append([]ModelRoute(nil), hivyModel.Routes...)
}

func (r *Registry) CanonicalModelsForProviders(providerIDs []string) []RoutedModel {
	allowed := map[string]bool{}
	for _, providerID := range providerIDs {
		allowed[providerID] = true
	}

	byID := map[string]RoutedModel{}
	for _, hivyModel := range supportedHivyModels {
		for _, route := range hivyModel.Routes {
			if !allowed[route.ProviderID] {
				continue
			}
			resolved, ok := r.ResolveModel(route.ProviderID, hivyModel.ID)
			if !ok {
				continue
			}
			rm := byID[hivyModel.ID]
			if rm.ID == "" {
				rm.Model = resolved.Model
				rm.ProviderIDs = []string{}
			}
			rm.ProviderIDs = appendIfMissing(rm.ProviderIDs, route.ProviderID)
			byID[hivyModel.ID] = rm
		}
	}

	out := make([]RoutedModel, 0, len(byID))
	for _, rm := range byID {
		out = append(out, rm)
	}
	sortRoutedModels(out)
	return out
}

func sortRoutedModels(models []RoutedModel) {
	for i := range models {
		sort.Strings(models[i].ProviderIDs)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
}

func (r *Registry) ValidateCanonicalModel(canonicalID string) error {
	if canonicalID == "" {
		return nil
	}
	if _, ok := r.CanonicalModel(canonicalID); ok {
		return nil
	}
	return fmt.Errorf("model %q is not in the catalog", canonicalID)
}

func (r *Registry) providerModel(providerID, modelID string) (Model, bool) {
	provider, ok := r.GetProvider(providerID)
	if !ok {
		return Model{}, false
	}
	mdl, ok := provider.Models[modelID]
	return mdl, ok
}

func providerIDsForRoutes(routes []ModelRoute) []string {
	out := make([]string, 0, len(routes))
	for _, route := range routes {
		out = appendIfMissing(out, route.ProviderID)
	}
	sort.Strings(out)
	return out
}

func appendIfMissing(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
