package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

const (
	MCPServerScopePersonal = "personal"
	MCPServerScopeOrg      = "org"

	MCPTransportStreamableHTTP = "streamable_http"
	MCPTransportSSE            = "sse"

	MCPAuthTypeNone                   = "none"
	MCPAuthTypeStaticBearer           = "static_bearer"
	MCPAuthTypeStaticHeader           = "static_header"
	MCPAuthTypeOAuthAuthorizationCode = "oauth_authorization_code"
	// #nosec G101 -- this is an OAuth grant-type identifier, not a credential.
	MCPAuthTypeOAuthClientCredentials = "oauth_client_credentials"

	MCPAuthorizationPolicyNone            = "none"
	MCPAuthorizationPolicyUserRequired    = "user_required"
	MCPAuthorizationPolicyServiceRequired = "service_required"
	MCPAuthorizationPolicyPreferUser      = "prefer_user"
	MCPAuthorizationPolicyPreferService   = "prefer_service"

	MCPServerStatusActive   = "active"
	MCPServerStatusDisabled = "disabled"

	MCPPrincipalUser       = "user"
	MCPPrincipalOrgService = "org_service"

	MCPAuthorizationStatusActive  = "active"
	MCPAuthorizationStatusExpired = "expired"
	MCPAuthorizationStatusRevoked = "revoked"

	MCPAgentGrantEnabled  = "enabled"
	MCPAgentGrantDisabled = "disabled"
)

// MCPServer is an org-owned or personal remote MCP server definition. Personal
// definitions are visible only to OwnerUserID; organization definitions can be
// provisioned to teams and agents without sharing a member's authorization.
type MCPServer struct {
	ID                  uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID               uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org                 Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	Scope               string     `gorm:"type:text;not null;index"`
	OwnerUserID         *uuid.UUID `gorm:"type:uuid;index"`
	OwnerUser           *User      `gorm:"foreignKey:OwnerUserID;constraint:OnDelete:CASCADE"`
	Slug                string     `gorm:"type:text;not null"`
	Name                string     `gorm:"type:text;not null"`
	Description         string     `gorm:"type:text;not null;default:''"`
	URL                 string     `gorm:"column:url;type:text;not null"`
	Transport           string     `gorm:"type:text;not null;default:'streamable_http'"`
	AuthType            string     `gorm:"type:text;not null;default:'none'"`
	AuthorizationPolicy string     `gorm:"type:text;not null;default:'none'"`
	HeaderName          string     `gorm:"type:text;not null;default:''"`
	OAuthMetadata       JSON       `gorm:"column:oauth_metadata;type:jsonb;not null;default:'{}'"`
	Status              string     `gorm:"type:text;not null;default:'active';index"`
	CreatedByUserID     *uuid.UUID `gorm:"type:uuid;index"`
	CreatedByUser       *User      `gorm:"foreignKey:CreatedByUserID;constraint:OnDelete:SET NULL"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (MCPServer) TableName() string { return "mcp_servers" }

// MCPAuthorization stores one encrypted credential set for a user or for the
// organization service identity. Metadata is intentionally non-secret and is
// safe to expose after response shaping; CredentialsEncrypted is never exposed.
type MCPAuthorization struct {
	ID                   uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID                uuid.UUID      `gorm:"type:uuid;not null;index"`
	Org                  Org            `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	MCPServerID          uuid.UUID      `gorm:"type:uuid;not null;index"`
	MCPServer            MCPServer      `gorm:"foreignKey:MCPServerID;constraint:OnDelete:CASCADE"`
	PrincipalType        string         `gorm:"type:text;not null;index"`
	PrincipalUserID      *uuid.UUID     `gorm:"type:uuid;index"`
	PrincipalUser        *User          `gorm:"foreignKey:PrincipalUserID;constraint:OnDelete:CASCADE"`
	AuthType             string         `gorm:"type:text;not null"`
	CredentialsEncrypted []byte         `gorm:"type:bytea;not null"`
	ClientID             string         `gorm:"type:text;not null;default:''"`
	Scopes               pq.StringArray `gorm:"type:text[];not null;default:'{}'"`
	TokenType            string         `gorm:"type:text;not null;default:''"`
	ExpiresAt            *time.Time     `gorm:"type:timestamptz"`
	RefreshExpiresAt     *time.Time     `gorm:"type:timestamptz"`
	Status               string         `gorm:"type:text;not null;default:'active';index"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (MCPAuthorization) TableName() string { return "mcp_authorizations" }

// MCPOAuthState binds one authorization-code callback to a server, org, user,
// principal, and PKCE verifier. The verifier is encrypted with the same
// internal secret key as credentials and the state is single-use.
type MCPOAuthState struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID             uuid.UUID  `gorm:"type:uuid;not null;index"`
	MCPServerID       uuid.UUID  `gorm:"type:uuid;not null;index"`
	UserID            uuid.UUID  `gorm:"type:uuid;not null;index"`
	PrincipalType     string     `gorm:"type:text;not null"`
	StateHash         []byte     `gorm:"type:bytea;not null;uniqueIndex"`
	EncryptedVerifier []byte     `gorm:"type:bytea;not null"`
	RedirectAfter     string     `gorm:"type:text;not null;default:''"`
	ExpiresAt         time.Time  `gorm:"type:timestamptz;not null;index"`
	UsedAt            *time.Time `gorm:"type:timestamptz"`
	CreatedAt         time.Time
}

func (MCPOAuthState) TableName() string { return "mcp_oauth_states" }

// TeamMCPServer grants one organization MCP server to all agents in a team.
type TeamMCPServer struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID         uuid.UUID  `gorm:"type:uuid;not null;index"`
	TeamID        uuid.UUID  `gorm:"type:uuid;not null;index"`
	Team          Team       `gorm:"foreignKey:TeamID;constraint:OnDelete:CASCADE"`
	MCPServerID   uuid.UUID  `gorm:"type:uuid;not null;index"`
	MCPServer     MCPServer  `gorm:"foreignKey:MCPServerID;constraint:OnDelete:CASCADE"`
	GrantedBy     *uuid.UUID `gorm:"type:uuid"`
	GrantedByUser *User      `gorm:"foreignKey:GrantedBy;constraint:OnDelete:SET NULL"`
	CreatedAt     time.Time
}

func (TeamMCPServer) TableName() string { return "team_mcp_servers" }

// AgentMCPServer records a direct org-server grant or disables an inherited
// team grant for one agent.
type AgentMCPServer struct {
	OrgID       uuid.UUID  `gorm:"type:uuid;not null;index"`
	AgentID     uuid.UUID  `gorm:"type:uuid;not null;primaryKey"`
	Agent       Agent      `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`
	MCPServerID uuid.UUID  `gorm:"type:uuid;not null;primaryKey;index"`
	MCPServer   MCPServer  `gorm:"foreignKey:MCPServerID;constraint:OnDelete:CASCADE"`
	State       string     `gorm:"type:text;not null"`
	UpdatedBy   *uuid.UUID `gorm:"type:uuid"`
	UpdatedUser *User      `gorm:"foreignKey:UpdatedBy;constraint:OnDelete:SET NULL"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (AgentMCPServer) TableName() string { return "agent_mcp_servers" }

// UserAgentMCPServer attaches a personal server to one agent for its owner.
// The user and server ownership constraints are also enforced by the service.
type UserAgentMCPServer struct {
	OrgID       uuid.UUID `gorm:"type:uuid;not null;index"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;primaryKey"`
	User        User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	AgentID     uuid.UUID `gorm:"type:uuid;not null;primaryKey"`
	Agent       Agent     `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`
	MCPServerID uuid.UUID `gorm:"type:uuid;not null;primaryKey;index"`
	MCPServer   MCPServer `gorm:"foreignKey:MCPServerID;constraint:OnDelete:CASCADE"`
	CreatedAt   time.Time
}

func (UserAgentMCPServer) TableName() string { return "user_agent_mcp_servers" }
