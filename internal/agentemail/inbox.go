package agentemail

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

var (
	ErrAgentNotFound = errors.New("agent not found")
	ErrNotConfigured = errors.New("agent email is not configured")
)

type Inbox struct {
	Address      string
	MessageCount int64
}

// GetInbox returns nil when the agent has not provisioned an inbox yet.
func GetInbox(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID, domain string) (*Inbox, error) {
	agent, err := loadInboxAgent(ctx, db, orgID, agentID, false)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(agent.EmailInboxLocalPart) == "" {
		return nil, nil
	}
	return inboxSummary(ctx, db, agent, domain)
}

// ProvisionInbox creates a stable address once and returns the existing address
// on repeated calls. A row lock makes concurrent requests idempotent.
func ProvisionInbox(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID, domain string) (Inbox, bool, error) {
	domain = normalizeInboxDomain(domain)
	if domain == "" {
		return Inbox{}, false, ErrNotConfigured
	}

	var agent model.Agent
	created := false
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		agent, err = loadInboxAgent(ctx, tx, orgID, agentID, true)
		if err != nil {
			return err
		}
		if strings.TrimSpace(agent.EmailInboxLocalPart) != "" {
			return nil
		}
		localPart, err := generateInboxLocalPart(agent.Name)
		if err != nil {
			return err
		}
		result := tx.WithContext(ctx).
			Model(&model.Agent{}).
			Where("id = ? AND org_id = ? AND email_inbox_local_part = ?", agent.ID, orgID, "").
			Update("email_inbox_local_part", localPart)
		if result.Error != nil {
			return fmt.Errorf("provision agent inbox: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("provision agent inbox: expected one updated agent, got %d", result.RowsAffected)
		}
		revision := tx.WithContext(ctx).
			Model(&model.Org{}).
			Where("id = ?", orgID).
			UpdateColumn("mcp_config_version", gorm.Expr("mcp_config_version + 1"))
		if revision.Error != nil {
			return fmt.Errorf("advance MCP config version for agent inbox: %w", revision.Error)
		}
		if revision.RowsAffected != 1 {
			return fmt.Errorf("advance MCP config version for agent inbox: expected one updated org, got %d", revision.RowsAffected)
		}
		agent.EmailInboxLocalPart = localPart
		created = true
		return nil
	})
	if err != nil {
		return Inbox{}, false, err
	}
	inbox, err := inboxSummary(ctx, db, agent, domain)
	if err != nil {
		return Inbox{}, false, err
	}
	return *inbox, created, nil
}

func loadInboxAgent(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID, forUpdate bool) (model.Agent, error) {
	var agent model.Agent
	query := db.WithContext(ctx).
		Select("id", "org_id", "name", "email_inbox_local_part").
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived")
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Agent{}, ErrAgentNotFound
		}
		return model.Agent{}, fmt.Errorf("load agent inbox: %w", err)
	}
	return agent, nil
}

func inboxSummary(ctx context.Context, db *gorm.DB, agent model.Agent, domain string) (*Inbox, error) {
	domain = normalizeInboxDomain(domain)
	if domain == "" {
		return nil, ErrNotConfigured
	}
	var messageCount int64
	if err := db.WithContext(ctx).
		Model(&model.AgentEmailMessage{}).
		Where("org_id = ? AND agent_id = ? AND direction = ?", *agent.OrgID, agent.ID, model.AgentEmailDirectionInbound).
		Count(&messageCount).Error; err != nil {
		return nil, fmt.Errorf("count agent inbox messages: %w", err)
	}
	return &Inbox{
		Address:      agent.EmailInboxLocalPart + "@" + domain,
		MessageCount: messageCount,
	}, nil
}

func generateInboxLocalPart(agentName string) (string, error) {
	buf := make([]byte, 5) // 40 random bits -> 8 unpadded base32 characters.
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate agent inbox suffix: %w", err)
	}
	suffix := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf))
	name := inboxSlug(agentName)
	// RFC 5321 caps an email local part at 64 octets. Keep room for '-' + suffix.
	if len(name) > 55 {
		name = name[:55]
	}
	return name + "-" + suffix, nil
}

func inboxSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			builder.WriteRune(character)
			lastDash = false
			continue
		}
		if !lastDash && builder.Len() > 0 {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "agent"
	}
	return result
}

func normalizeInboxDomain(domain string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(domain), "."))
}
