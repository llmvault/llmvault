package handler

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *GatewayExternalHandler) loadEmployee(w http.ResponseWriter, r *http.Request, orgID uuid.UUID) (*model.Employee, bool) {
	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee id"})
		return nil, false
	}
	var agent model.Employee
	if err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").
		First(&agent).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
		return nil, false
	}
	return &agent, true
}

func (h *GatewayExternalHandler) loadRouteForEmployee(w http.ResponseWriter, r *http.Request) (model.EmployeeGatewayRoute, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeMissingOrg(w)
		return model.EmployeeGatewayRoute{}, false
	}
	agent, ok := h.loadEmployee(w, r, org.ID)
	if !ok {
		return model.EmployeeGatewayRoute{}, false
	}
	routeID, err := uuid.Parse(chi.URLParam(r, "routeID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid route_id"})
		return model.EmployeeGatewayRoute{}, false
	}
	var route model.EmployeeGatewayRoute
	err = h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND employee_id = ? AND config->>'adapter' = ? AND revoked_at IS NULL", routeID, org.ID, agent.ID, gateway.ExternalAdapterName).
		First(&route).Error
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "gateway route not found"})
		return model.EmployeeGatewayRoute{}, false
	}
	return route, true
}

func (h *GatewayExternalHandler) loadExternalRoute(r *http.Request, routeID uuid.UUID) (model.EmployeeGatewayRoute, error) {
	var route model.EmployeeGatewayRoute
	err := h.db.WithContext(r.Context()).
		Where("id = ? AND config->>'adapter' = ?", routeID, gateway.ExternalAdapterName).
		First(&route).Error
	return route, err
}

func (h *GatewayExternalHandler) externalRouteConfig(callbackURL, hash, prefix, secret string, rotatedAt time.Time) (model.JSON, error) {
	if h.encKey == nil {
		return nil, fmt.Errorf("gateway encryption key is not configured")
	}
	encrypted, err := h.encKey.EncryptString(secret)
	if err != nil {
		return nil, err
	}
	return model.JSON{
		"adapter":             gateway.ExternalAdapterName,
		"callback_url":        callbackURL,
		"secret_hash":         hash,
		"encrypted_secret":    base64.StdEncoding.EncodeToString(encrypted),
		"secret_prefix":       prefix,
		"secret_rotated_at":   rotatedAt.Format(time.RFC3339),
		"callback_signature":  "hmac-sha256",
		"callback_body_shape": "external_gateway_v1",
	}, nil
}

func (h *GatewayExternalHandler) routeResponse(route model.EmployeeGatewayRoute, secret string) gatewayRouteResponse {
	return gatewayRouteResponse{
		ID:           route.ID.String(),
		Name:         route.Name,
		Provider:     route.Provider,
		InboundURL:   h.publicURL + "/incoming/gateways/external/" + route.ID.String(),
		CallbackURL:  externalRouteCallbackURL(route),
		Secret:       secret,
		SecretPrefix: externalRouteSecretPrefix(route),
		Enabled:      route.Enabled,
		CreatedAt:    route.CreatedAt,
		UpdatedAt:    route.UpdatedAt,
		RevokedAt:    route.RevokedAt,
	}
}

func validateGatewayRouteInput(provider, callbackURL string) (string, string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return "", "", fmt.Errorf("provider is required")
	}
	if provider == gateway.ExternalProvider || provider == gateway.HTTPProvider || provider == gateway.SlackProvider {
		return "", "", fmt.Errorf("provider %q is reserved", provider)
	}
	callbackURL = strings.TrimSpace(callbackURL)
	parsed, err := url.Parse(callbackURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", fmt.Errorf("callback_url must be an absolute URL")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", "", fmt.Errorf("callback_url must use http or https")
	}
	return provider, callbackURL, nil
}

func externalRouteCallbackURL(route model.EmployeeGatewayRoute) string {
	value, _ := route.Config["callback_url"].(string)
	return strings.TrimSpace(value)
}

func externalRouteSecretHash(route model.EmployeeGatewayRoute) string {
	value, _ := route.Config["secret_hash"].(string)
	return strings.TrimSpace(value)
}

func externalRouteSecretPrefix(route model.EmployeeGatewayRoute) string {
	value, _ := route.Config["secret_prefix"].(string)
	return strings.TrimSpace(value)
}

func decryptExternalRouteSecret(encKey *crypto.SymmetricKey, route model.EmployeeGatewayRoute) (string, error) {
	encoded, _ := route.Config["encrypted_secret"].(string)
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", fmt.Errorf("decode external gateway route secret: %w", err)
	}
	secret, err := encKey.DecryptString(raw)
	if err != nil {
		return "", fmt.Errorf("decrypt external gateway route secret: %w", err)
	}
	return secret, nil
}

func absoluteRuntimeURL(runtimeURL, pathOrURL string) string {
	if strings.TrimSpace(pathOrURL) == "" {
		return ""
	}
	if strings.HasPrefix(pathOrURL, "http://") || strings.HasPrefix(pathOrURL, "https://") {
		return pathOrURL
	}
	return strings.TrimRight(runtimeURL, "/") + "/" + strings.TrimLeft(pathOrURL, "/")
}
