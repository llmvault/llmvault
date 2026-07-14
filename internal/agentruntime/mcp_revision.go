package agentruntime

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// MCPConfigVersion returns the org-wide monotonic revision maintained by
// database triggers for MCP definitions, authorizations, and assignments.
func MCPConfigVersion(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (int64, error) {
	if db == nil || orgID == uuid.Nil {
		return 0, nil
	}
	var version int64
	if err := db.WithContext(ctx).Table("orgs").
		Select("mcp_config_version").
		Where("id = ?", orgID).
		Scan(&version).Error; err != nil {
		return 0, fmt.Errorf("load MCP config version: %w", err)
	}
	return version, nil
}
