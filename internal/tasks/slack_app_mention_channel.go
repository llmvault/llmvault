package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
	"github.com/usehivy/hivy/internal/slackworkflow"
)

var errSlackChannelNotConfigured = errors.New("slack channel is not routed to a hivy agent")

const slackChannelNotConfiguredMessage = "This Slack channel is not routed to a Hivy agent yet. An admin can configure an external resource route in the Hivy dashboard to enable the assistant here."

// resolveTeamAndAgent preserves thread affinity before resolving a new inbound
// Slack resource route. Resource routing is deterministic: no channel and no
// LLM agent chooser are involved.
func (h *SlackAppMentionHandler) resolveTeamAndAgent(ctx context.Context, row *model.SlackThreadEvent, client slackapp.Client) (model.Team, model.Agent, error) {
	if row.SessionID != nil && *row.SessionID != uuid.Nil {
		session, err := h.loadActiveSlackSession(ctx, row.OrgID, *row.SessionID)
		if err != nil {
			return model.Team{}, model.Agent{}, fmt.Errorf("load slack continuation session: %w", err)
		}
		team, err := h.loadSlackSessionTeam(ctx, row.OrgID, session.TeamID)
		if err != nil {
			return model.Team{}, model.Agent{}, err
		}
		agent, err := h.loadAgent(ctx, row.OrgID, session.AgentID)
		if err != nil {
			return model.Team{}, model.Agent{}, err
		}
		row.ResolvedTeamID, row.AgentID = &team.ID, &agent.ID
		_ = slackworkflow.RecordRouteResolved(ctx, h.db, row.ID, team.ID, agent.ID)
		return team, agent, nil
	}
	if row.TriggerID != nil && *row.TriggerID != uuid.Nil {
		agent, err := h.loadSlackTriggerAgent(ctx, row.OrgID, *row.TriggerID)
		if err != nil {
			return model.Team{}, model.Agent{}, err
		}
		team, err := h.loadSlackSessionTeam(ctx, row.OrgID, agent.TeamID)
		if err != nil {
			return model.Team{}, model.Agent{}, err
		}
		row.ResolvedTeamID, row.AgentID = &team.ID, &agent.ID
		_ = slackworkflow.RecordRouteResolved(ctx, h.db, row.ID, team.ID, agent.ID)
		return team, agent, nil
	}
	route, found, err := h.findSlackExternalResourceRoute(ctx, *row)
	if err != nil {
		return model.Team{}, model.Agent{}, err
	}
	if !found {
		if err := h.replyChannelNotConfigured(ctx, row, client); err != nil {
			return model.Team{}, model.Agent{}, err
		}
		return model.Team{}, model.Agent{}, errSlackChannelNotConfigured
	}
	team, err := h.loadSlackSessionTeam(ctx, row.OrgID, route.TeamID)
	if err != nil {
		return model.Team{}, model.Agent{}, err
	}
	agent, err := h.loadAgent(ctx, row.OrgID, route.AgentID)
	if err != nil {
		return model.Team{}, model.Agent{}, err
	}
	row.ResolvedTeamID, row.AgentID = &team.ID, &agent.ID
	_ = slackworkflow.RecordRouteResolved(ctx, h.db, row.ID, team.ID, agent.ID)
	return team, agent, nil
}

func (h *SlackAppMentionHandler) replyChannelNotConfigured(ctx context.Context, row *model.SlackThreadEvent, client slackapp.Client) error {
	replyTS, err := slackapp.PostThreadReply(ctx, client, row.SlackChannelID, row.ThreadTS, slackChannelNotConfiguredMessage)
	if err != nil {
		return fmt.Errorf("post slack channel-not-configured notice: %w", err)
	}
	return slackworkflow.RecordReplySent(ctx, h.db, row.ID, replyTS)
}

func (h *SlackAppMentionHandler) loadSlackTriggerAgent(ctx context.Context, orgID, triggerID uuid.UUID) (model.Agent, error) {
	var trigger model.AgentTrigger
	err := h.db.WithContext(ctx).Where("id = ? AND org_id = ? AND enabled = true", triggerID, orgID).First(&trigger).Error
	if err != nil {
		return model.Agent{}, fmt.Errorf("load slack trigger: %w", err)
	}
	return h.loadAgent(ctx, orgID, trigger.AgentID)
}

func (h *SlackAppMentionHandler) findSlackExternalResourceRoute(ctx context.Context, row model.SlackThreadEvent) (model.TeamExternalResourceRoute, bool, error) {
	var route model.TeamExternalResourceRoute
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND connection_id = ? AND resource_type = ? AND resource_key = ?", row.OrgID, row.ConnectionID, "slack_channel", row.SlackChannelID).
		First(&route).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.TeamExternalResourceRoute{}, false, nil
	}
	if err != nil {
		return model.TeamExternalResourceRoute{}, false, fmt.Errorf("load slack external resource route: %w", err)
	}
	return route, true, nil
}

func (h *SlackAppMentionHandler) findOrCreateSlackSession(ctx context.Context, row *model.SlackThreadEvent, team model.Team, agent model.Agent) (model.Session, error) {
	if row.SessionID != nil && *row.SessionID != uuid.Nil {
		session, err := h.loadActiveSlackSession(ctx, row.OrgID, *row.SessionID)
		if err != nil {
			return model.Session{}, fmt.Errorf("load slack continuation session: %w", err)
		}
		_ = slackworkflow.RecordSessionResolved(ctx, h.db, row.ID, session.ID)
		return session, nil
	}
	key := slackSessionResourceKey(*row)
	var session model.Session
	err := h.db.WithContext(ctx).Where("org_id = ? AND source = ? AND source_id = ? AND source_resource_key = ? AND status = ?", row.OrgID, model.SessionSourceExternal, row.ConnectionID, key, "active").First(&session).Error
	if err == nil {
		_ = slackworkflow.RecordSessionResolved(ctx, h.db, row.ID, session.ID)
		return session, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Session{}, fmt.Errorf("load slack session: %w", err)
	}
	var generation int64
	if err := h.db.WithContext(ctx).Model(&model.Session{}).Where("org_id = ? AND source = ? AND source_id = ? AND source_resource_key = ?", row.OrgID, model.SessionSourceExternal, row.ConnectionID, key).Count(&generation).Error; err != nil {
		return model.Session{}, fmt.Errorf("count slack sessions: %w", err)
	}
	connID := row.ConnectionID
	session = model.Session{ID: stableSlackSessionID(*row, generation), OrgID: row.OrgID, TeamID: team.ID, AgentID: agent.ID, Model: agent.Model, ReasoningEffort: sessionReasoningEffort(agent), Source: model.SessionSourceExternal, SourceID: &connID, SourceResourceKey: key, Status: "active", AgentTurnStatus: model.SessionAgentTurnIdle, IntegrationScopes: model.JSON{}}
	if err := h.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&session).Error; err != nil {
		return model.Session{}, fmt.Errorf("create slack session: %w", err)
	}
	if err := h.db.WithContext(ctx).First(&session, "id = ?", session.ID).Error; err != nil {
		return model.Session{}, fmt.Errorf("reload slack session: %w", err)
	}
	_ = slackworkflow.RecordSessionResolved(ctx, h.db, row.ID, session.ID)
	return session, nil
}

func (h *SlackAppMentionHandler) loadActiveSlackSession(ctx context.Context, orgID, sessionID uuid.UUID) (model.Session, error) {
	var session model.Session
	err := h.db.WithContext(ctx).Where("id = ? AND org_id = ? AND source = ? AND status = ?", sessionID, orgID, model.SessionSourceExternal, "active").First(&session).Error
	return session, err
}

func (h *SlackAppMentionHandler) loadSlackSessionTeam(ctx context.Context, orgID, teamID uuid.UUID) (model.Team, error) {
	var team model.Team
	err := h.db.WithContext(ctx).Where("id = ? AND org_id = ? AND archived_at IS NULL", teamID, orgID).First(&team).Error
	if err != nil {
		return model.Team{}, fmt.Errorf("load slack session team: %w", err)
	}
	return team, nil
}

func slackSessionResourceKey(row model.SlackThreadEvent) string {
	return strings.Join([]string{slackapp.Provider, row.ConnectionID.String(), row.SlackTeamID, row.SlackChannelID, row.ThreadTS}, ":")
}

func stableSlackSessionID(row model.SlackThreadEvent, generation int64) uuid.UUID {
	key := "hivy:slack-session:" + slackSessionResourceKey(row)
	if generation > 0 {
		key = fmt.Sprintf("%s:gen:%d", key, generation)
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(key))
}
