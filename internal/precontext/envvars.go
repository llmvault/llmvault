package precontext

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

const (
	envVarsSectionTitle = "## Environment variables"
	envVarsPreamble     = "The following environment variables are available in this team's sandbox sessions. You may reference them by NAME in commands and code (e.g. $STRIPE_API_KEY). You must NEVER print, echo, log, or reveal their VALUES in any response, message, file, or memory — not even partially, encoded, or transformed. If a user asks for a value, refuse and point them to the team's environment settings."
)

// fetchEnvVarsSection renders the team's environment variables as an
// awareness section: names and descriptions only, prefixed with the mandatory
// never-reveal-values instruction. Values never flow through this path — the
// EnvVarLister contract cannot even represent them. A team with no env vars
// gets no section, and a lister failure only costs this section (the Build
// degrade pattern), never the rest of precontext. The section carries no
// per-section or per-line byte budget; only the shared TotalBudgetBytes cap
// applied by joinSections bounds it.
func (s *Service) fetchEnvVarsSection(ctx context.Context, req Request) (string, error) {
	if isNilValue(s.cfg.EnvVars) || req.OrgID == uuid.Nil || req.TeamID == uuid.Nil {
		return "", nil
	}
	vars, err := s.cfg.EnvVars.TeamEnvVars(ctx, req.OrgID, req.TeamID)
	if err != nil {
		return "", fmt.Errorf("list team env vars: %w", err)
	}
	lines := make([]string, 0, len(vars))
	for _, v := range vars {
		name := cleanText(v.Name)
		if name == "" {
			continue
		}
		description := cleanText(v.Description)
		if description == "" {
			description = "(no description)"
		}
		lines = append(lines, "- "+name+" — "+description)
	}
	if len(lines) == 0 {
		return "", nil
	}
	return envVarsSectionTitle + "\n" + envVarsPreamble + "\n" + strings.Join(lines, "\n"), nil
}

// GormEnvVarLister is the DB-backed EnvVarLister used by cmd/server wiring.
// Its query selects ONLY the name and description columns — the encrypted
// value column is never read on this path, by construction.
type GormEnvVarLister struct {
	db *gorm.DB
}

func NewGormEnvVarLister(db *gorm.DB) *GormEnvVarLister {
	return &GormEnvVarLister{db: db}
}

func (l *GormEnvVarLister) TeamEnvVars(ctx context.Context, orgID, teamID uuid.UUID) ([]EnvVarDoc, error) {
	if l == nil || l.db == nil || orgID == uuid.Nil || teamID == uuid.Nil {
		return nil, nil
	}
	var docs []EnvVarDoc
	if err := l.db.WithContext(ctx).
		Model(&model.TeamEnvVar{}).
		Select("name", "description").
		Where("org_id = ? AND team_id = ?", orgID, teamID).
		Order("name ASC").
		Find(&docs).Error; err != nil {
		return nil, fmt.Errorf("load team env var docs: %w", err)
	}
	return docs, nil
}
