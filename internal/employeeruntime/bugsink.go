package employeeruntime

import (
	"context"
	"net/url"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

const bugsinkProvider = "bugsink"
const glitchTipProvider = "glitchtip"

// BugsinkDashboardBaseURL returns the real Bugsink instance base URL attached
// to the employee. It deliberately does not return HIVY_BUGSINK_URL, because
// HIVY_BUGSINK_URL is the Hivy proxy URL used for API calls.
func BugsinkDashboardBaseURL(ctx context.Context, db *gorm.DB, orgID uuid.UUID, agent model.Employee) string {
	return providerDashboardBaseURL(ctx, db, orgID, agent, bugsinkProvider)
}

// GlitchTipDashboardBaseURL returns the real GlitchTip instance base URL attached
// to the employee. It deliberately does not return HIVY_GLITCHTIP_URL, because
// HIVY_GLITCHTIP_URL is the Hivy proxy URL used for API calls.
func GlitchTipDashboardBaseURL(ctx context.Context, db *gorm.DB, orgID uuid.UUID, agent model.Employee) string {
	return providerDashboardBaseURL(ctx, db, orgID, agent, glitchTipProvider)
}

func providerDashboardBaseURL(ctx context.Context, db *gorm.DB, orgID uuid.UUID, agent model.Employee, provider string) string {
	if db == nil || orgID == uuid.Nil {
		return ""
	}
	connectionIDs := connectionIDsFromAgentIntegrations(agent.Integrations)
	if len(connectionIDs) == 0 {
		return ""
	}

	var conn model.Connection
	if err := db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Where("connections.id IN ? AND connections.org_id = ? AND connections.revoked_at IS NULL AND integrations.provider = ?", connectionIDs, orgID, provider).
		Order("connections.created_at ASC").
		First(&conn).Error; err != nil {
		return ""
	}
	return DashboardBaseURLFromConnection(conn)
}

func BugsinkDashboardBaseURLFromConnection(conn model.Connection) string {
	return DashboardBaseURLFromConnection(conn)
}

func GlitchTipDashboardBaseURLFromConnection(conn model.Connection) string {
	return DashboardBaseURLFromConnection(conn)
}

func DashboardBaseURLFromConnection(conn model.Connection) string {
	connectionConfig, ok := conn.Meta["connection_config"].(map[string]any)
	if !ok {
		if typed, ok := conn.Meta["connection_config"].(model.JSON); ok {
			connectionConfig = typed
		}
	}
	raw, _ := connectionConfig["baseUrl"].(string)
	return normalizeDashboardBaseURL(raw)
}

func connectionIDsFromAgentIntegrations(integrations model.JSON) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(integrations))
	for rawID := range integrations {
		id, err := uuid.Parse(rawID)
		if err == nil {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	})
	return ids
}

func normalizeDashboardBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}
