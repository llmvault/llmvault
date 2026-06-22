package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	AgentSandboxUpgradeStatusQueued    = "queued"
	AgentSandboxUpgradeStatusRunning   = "running"
	AgentSandboxUpgradeStatusSucceeded = "succeeded"
	AgentSandboxUpgradeStatusFailed    = "failed"

	AgentSandboxUpgradePhaseQueued      = "queued"
	AgentSandboxUpgradePhaseBackup      = "backup"
	AgentSandboxUpgradePhaseCreatingNew = "creating_new"
	AgentSandboxUpgradePhaseRestore     = "restore"
	AgentSandboxUpgradePhaseRestartNew  = "restart_new"
	AgentSandboxUpgradePhaseSync        = "sync"
	AgentSandboxUpgradePhaseDrainingOld = "draining_old"
	AgentSandboxUpgradePhasePausingOld  = "pausing_old"
	AgentSandboxUpgradePhaseCleanupOld  = "cleanup_old"
	AgentSandboxUpgradePhaseCompleted   = "completed"
	AgentSandboxUpgradePhaseFailed      = "failed"
)

type AgentSandboxUpgrade struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	OrgID uuid.UUID `gorm:"type:uuid;not null;index"`
	Org   Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`

	AgentID uuid.UUID `gorm:"type:uuid;not null;index"`
	Agent   Agent     `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`

	OldSandboxID *uuid.UUID `gorm:"type:uuid;index"`
	OldSandbox   *Sandbox   `gorm:"foreignKey:OldSandboxID;constraint:OnDelete:SET NULL"`
	NewSandboxID *uuid.UUID `gorm:"type:uuid;index"`
	NewSandbox   *Sandbox   `gorm:"foreignKey:NewSandboxID;constraint:OnDelete:SET NULL"`

	Status string `gorm:"not null;default:'queued';size:32;index"`
	Phase  string `gorm:"not null;default:'queued';size:64"`

	BackupKey    *string
	BackupSHA256 *string
	BackupBytes  int64 `gorm:"not null;default:0"`

	ErrorMessage *string
	CompletedAt  *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (AgentSandboxUpgrade) TableName() string { return "agent_sandbox_upgrades" }
