package agentruntime

import (
	"context"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/skills"
)

// compileAutoLoadSkills normalizes an agent's auto_load_skills into the runtime
// contract form ([]model.AutoLoadSkill with a non-nil files array) and drops any
// entry naming a skill that does not resolve for resolveAgent. A bad slug is
// logged loudly and skipped — a config push must never fail the whole session
// for one unresolvable auto-load skill. resolveAgent is the agent whose skill
// entitlements gate the entries (the sub-agent's parent for sub-agent
// definitions, since sub-agents inherit the parent's plugins).
func compileAutoLoadSkills(ctx context.Context, db *gorm.DB, resolveAgent *model.Agent, entries model.AutoLoadSkills) []model.AutoLoadSkill {
	normalized, err := model.NormalizeAutoLoadSkills(entries)
	if err != nil {
		// Stored rows are already normalized at write time; a normalization error
		// here means a malformed row. Log and emit nothing rather than fail.
		logging.FromContext(ctx).WarnContext(ctx, "agent runtime compile: invalid auto_load_skills dropped", "error", err)
		return nil
	}
	if len(normalized) == 0 {
		return nil
	}
	allowed, ok := resolvableSkillSlugs(ctx, db, resolveAgent)
	out := make([]model.AutoLoadSkill, 0, len(normalized))
	for _, entry := range normalized {
		if ok && !allowed[entry.Name] {
			logging.FromContext(ctx).WarnContext(ctx, "agent runtime compile: auto_load_skill not resolvable for agent; dropping",
				"skill", entry.Name)
			continue
		}
		out = append(out, entry)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// resolvableSkillSlugs returns the set of skill slugs the agent can load and
// ok=true when the set could be resolved. When db is nil or resolution fails it
// returns ok=false so callers skip validation (keep every entry) rather than
// drop everything.
func resolvableSkillSlugs(ctx context.Context, db *gorm.DB, agent *model.Agent) (map[string]bool, bool) {
	if db == nil || agent == nil {
		return nil, false
	}
	summaries, err := skills.AgentSkillSummaries(ctx, db, agent)
	if err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "agent runtime compile: could not resolve skills for auto_load validation; keeping entries", "error", err)
		return nil, false
	}
	allowed := make(map[string]bool, len(summaries))
	for _, summary := range summaries {
		allowed[summary.Name] = true
	}
	return allowed, true
}
