package registry

import "sort"

// CatalogModel is a canonical Hivy model and every provider route that
// supports it.
type CatalogModel struct {
	Model
	Routes []CatalogModelRoute
}

// CatalogModelRoute describes one provider-specific implementation of a
// canonical model.
type CatalogModelRoute struct {
	ProviderID       string
	ProviderName     string
	ProviderAPI      string
	ProviderDoc      string
	UpstreamModelID  string
	CanonicalModelID string
	Model            Model
}

// CatalogModels returns the complete canonical catalog regardless of which
// provider credentials are currently configured. Provider routes retain proxy
// priority, followed by any catalog-only routes not used by the proxy.
func (r *Registry) CatalogModels() []CatalogModel {
	out := make([]CatalogModel, 0, len(supportedHivyModels))
	for _, hivyModel := range supportedHivyModels {
		routes := catalogRoutes(hivyModel)
		catalogModel := CatalogModel{
			Model: Model{ID: hivyModel.ID, Name: hivyModel.ID},
		}
		for _, route := range routes {
			provider, ok := r.GetProvider(route.ProviderID)
			if !ok {
				continue
			}
			providerModel, ok := provider.Models[route.ModelID]
			if !ok {
				continue
			}
			if len(catalogModel.Routes) == 0 {
				catalogModel.Model = r.modelWithNewWindow(providerModel, hivyModel)
				catalogModel.ID = hivyModel.ID
				catalogModel.Hidden = false
			}
			catalogModel.Routes = append(catalogModel.Routes, CatalogModelRoute{
				ProviderID:       provider.ID,
				ProviderName:     provider.Name,
				ProviderAPI:      provider.API,
				ProviderDoc:      provider.Doc,
				UpstreamModelID:  route.ModelID,
				CanonicalModelID: route.CanonicalModelID,
				Model:            providerModel,
			})
		}
		out = append(out, catalogModel)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func catalogRoutes(model HivyModel) []ModelRoute {
	ordered := append([]ModelRoute(nil), model.ProxyRoutes...)
	if len(ordered) == 0 {
		ordered = append(ordered, model.Routes...)
	}
	for _, route := range model.Routes {
		if !containsModelRoute(ordered, route) {
			ordered = append(ordered, route)
		}
	}
	return ordered
}

func containsModelRoute(routes []ModelRoute, want ModelRoute) bool {
	for _, route := range routes {
		if route == want {
			return true
		}
	}
	return false
}
