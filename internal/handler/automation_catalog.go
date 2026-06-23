package handler

import (
	"net/http"

	"github.com/usehivy/hivy/internal/automationcatalog"
)

type AutomationCatalogHandler struct {
	triggerDir  string
	scheduleDir string
}

func NewAutomationCatalogHandler(triggerDir, scheduleDir string) *AutomationCatalogHandler {
	return &AutomationCatalogHandler{triggerDir: triggerDir, scheduleDir: scheduleDir}
}

type automationCatalogResponse struct {
	Data []automationcatalog.CatalogItem `json:"data"`
}

// ListTriggers handles GET /v1/catalog/triggers.
// @Summary List trigger automation catalog
// @Description Returns enabled one-click installable trigger templates from the file-backed global catalog.
// @Tags catalog
// @Produce json
// @Success 200 {object} automationCatalogResponse
// @Failure 500 {object} errorResponse
// @Router /v1/catalog/triggers [get]
func (h *AutomationCatalogHandler) ListTriggers(w http.ResponseWriter, r *http.Request) {
	items, err := automationcatalog.LoadTriggers(h.triggerDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load trigger catalog"})
		return
	}
	writeJSON(w, http.StatusOK, automationCatalogResponse{Data: enabledCatalogItems(items)})
}

// ListAutomations handles GET /v1/catalog/automations.
// @Summary List schedule automation catalog
// @Description Returns enabled one-click installable schedule templates from the file-backed global catalog.
// @Tags catalog
// @Produce json
// @Success 200 {object} automationCatalogResponse
// @Failure 500 {object} errorResponse
// @Router /v1/catalog/automations [get]
func (h *AutomationCatalogHandler) ListAutomations(w http.ResponseWriter, r *http.Request) {
	items, err := automationcatalog.LoadSchedules(h.scheduleDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load automation catalog"})
		return
	}
	writeJSON(w, http.StatusOK, automationCatalogResponse{Data: enabledCatalogItems(items)})
}

func enabledCatalogItems(items []automationcatalog.CatalogItem) []automationcatalog.CatalogItem {
	out := make([]automationcatalog.CatalogItem, 0, len(items))
	for _, item := range items {
		if item.Enabled {
			out = append(out, item)
		}
	}
	return out
}
