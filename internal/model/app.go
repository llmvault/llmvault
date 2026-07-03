package model

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// App statuses (apps plan §0).
const (
	AppStatusDraft     = "draft"
	AppStatusDeploying = "deploying"
	AppStatusRunning   = "running"
	AppStatusStopped   = "stopped"
	AppStatusFailed    = "failed"
)

// App is an agent-built web application bound to exactly ONE sheet: a static
// SPA plus a small backend hosted in a microsandbox. The app reads the sheet's
// structure and has full row CRUD on it through the internal app API — never
// schema mutation. EncryptedAppSecret holds the per-app runtime secret
// (AES-256-GCM with the sandbox encryption key, like
// sandboxes.encrypted_runtime_secret) that authenticates the app backend.
type App struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID              uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org                *Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	ChannelID          uuid.UUID  `gorm:"type:uuid;not null;index"`
	Channel            *Channel   `gorm:"foreignKey:ChannelID;constraint:OnDelete:CASCADE"`
	SheetID            uuid.UUID  `gorm:"type:uuid;not null;index"`
	Sheet              *Sheet     `gorm:"foreignKey:SheetID;constraint:OnDelete:CASCADE"`
	Slug               string     `gorm:"type:text;not null"`
	Name               string     `gorm:"type:text;not null"`
	Description        string     `gorm:"type:text;not null;default:''"`
	Icon               string     `gorm:"type:text;not null;default:''"`
	Alias              string     `gorm:"type:text;not null;default:''"` // microsandbox alias hostname stem; '' until claimed
	SandboxID          *uuid.UUID `gorm:"type:uuid"`                     // current app sandbox; nil until deployed
	Sandbox            *Sandbox   `gorm:"foreignKey:SandboxID;constraint:OnDelete:SET NULL"`
	EncryptedAppSecret []byte     `gorm:"type:bytea;not null"` // AES-256-GCM (SandboxEncKey) encrypted app runtime secret
	ActiveVersionID    *uuid.UUID `gorm:"type:uuid"`
	Status             string     `gorm:"type:text;not null;default:'draft'"`
	TemplateVersion    string     `gorm:"type:text;not null;default:''"`
	// PreviewURL is the public URL of the latest preview run of this app,
	// served from the BUILDER agent's sandbox (not an app sandbox). '' until
	// the builder registers one via the preview-env side channel.
	PreviewURL          string `gorm:"type:text;not null;default:''"`
	PreviewRegisteredAt *time.Time
	CreatedByAgentID    *uuid.UUID `gorm:"type:uuid"`
	CreatedByAgent      *Agent     `gorm:"foreignKey:CreatedByAgentID;constraint:OnDelete:SET NULL"`
	CreatedByUserID     *uuid.UUID `gorm:"type:uuid"`
	CreatedByUser       *User      `gorm:"foreignKey:CreatedByUserID;constraint:OnDelete:SET NULL"`
	SourceSessionID     *uuid.UUID `gorm:"type:uuid"`
	SourceSession       *Session   `gorm:"foreignKey:SourceSessionID;constraint:OnDelete:SET NULL"`
	ArchivedAt          *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (App) TableName() string { return "apps" }

// AppVersion is one immutable published build of an app: the source and
// bundle zips live at content-addressed object keys (new content = new key);
// rows are soft-archived for pruning and never overwritten.
type AppVersion struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	AppID            uuid.UUID  `gorm:"type:uuid;not null;index"`
	App              *App       `gorm:"foreignKey:AppID;constraint:OnDelete:CASCADE"`
	OrgID            uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org              *Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	SourceObjectKey  string     `gorm:"type:text;not null"`
	BundleObjectKey  string     `gorm:"type:text;not null"`
	SourceSHA256     string     `gorm:"column:source_sha256;type:text;not null"`
	BundleSHA256     string     `gorm:"column:bundle_sha256;type:text;not null"` // verified by appd before activation
	SourceBytes      int64      `gorm:"not null;default:0"`
	BundleBytes      int64      `gorm:"not null;default:0"`
	Notes            string     `gorm:"type:text;not null;default:''"`
	TemplateVersion  string     `gorm:"type:text;not null;default:''"`
	CreatedByAgentID *uuid.UUID `gorm:"type:uuid"`
	CreatedByAgent   *Agent     `gorm:"foreignKey:CreatedByAgentID;constraint:OnDelete:SET NULL"`
	SourceSessionID  *uuid.UUID `gorm:"type:uuid"`
	SourceSession    *Session   `gorm:"foreignKey:SourceSessionID;constraint:OnDelete:SET NULL"`
	ArchivedAt       *time.Time
	CreatedAt        time.Time
}

func (AppVersion) TableName() string { return "app_versions" }

// AppSecretPrefix distinguishes app runtime secrets from other bearer kinds.
const AppSecretPrefix = "hvapp_"

// GenerateAppSecret creates a new app runtime secret: "hvapp_" + 32 random
// bytes hex (mirrors GenerateAPIKey). The plaintext is never stored — it is
// encrypted at rest with the sandbox encryption key and decrypted only for
// sandbox injection and bearer verification.
func GenerateAppSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating app secret: %w", err)
	}
	return AppSecretPrefix + hex.EncodeToString(b), nil
}
