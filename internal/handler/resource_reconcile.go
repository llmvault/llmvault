package handler

import (
	"github.com/usehivy/hivy/internal/model"
)

func connectionDefaultResources(conn model.Connection) model.JSON {
	raw, ok := conn.Meta["resources"]
	if !ok || raw == nil {
		return model.JSON{}
	}
	switch typed := raw.(type) {
	case model.JSON:
		return typed
	case map[string]any:
		out := model.JSON{}
		for key, value := range typed {
			out[key] = value
		}
		return out
	default:
		return model.JSON{}
	}
}

func selectedResourceCount(resources model.JSON, resourceType string) int {
	if len(resources) == 0 || resourceType == "" {
		return 0
	}
	raw, ok := resources[resourceType]
	if !ok || raw == nil {
		return 0
	}
	switch typed := raw.(type) {
	case []any:
		return len(typed)
	case []map[string]any:
		return len(typed)
	default:
		return 0
	}
}
