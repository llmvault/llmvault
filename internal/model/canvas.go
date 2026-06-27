package model

import (
	"time"

	"github.com/google/uuid"
)

type CanvasProject struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID            uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org              *Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	Slug             string     `gorm:"type:text;not null;default:''"`
	Name             string     `gorm:"type:text;not null;default:''"`
	Description      string     `gorm:"type:text;not null;default:''"`
	CreatedByAgentID *uuid.UUID `gorm:"type:uuid"`
	CreatedByAgent   *Agent     `gorm:"foreignKey:CreatedByAgentID;constraint:OnDelete:SET NULL"`
	CreatedByUserID  *uuid.UUID `gorm:"type:uuid"`
	CreatedByUser    *User      `gorm:"foreignKey:CreatedByUserID;constraint:OnDelete:SET NULL"`
	ArchivedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (CanvasProject) TableName() string { return "canvas_projects" }

type CanvasArtifact struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID            uuid.UUID      `gorm:"type:uuid;not null;index"`
	Org              *Org           `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	CanvasProjectID  uuid.UUID      `gorm:"type:uuid;not null;index"`
	CanvasProject    *CanvasProject `gorm:"foreignKey:CanvasProjectID;constraint:OnDelete:CASCADE"`
	Slug             string         `gorm:"type:text;not null"`
	Type             string         `gorm:"type:text;not null"`
	Name             string         `gorm:"type:text;not null;default:''"`
	ManifestJSON     RawJSON        `gorm:"type:jsonb;not null;default:'{}'"`
	SourceSessionID  *uuid.UUID     `gorm:"type:uuid;index"`
	SourceSession    *Session       `gorm:"foreignKey:SourceSessionID;constraint:OnDelete:SET NULL"`
	CreatedByAgentID *uuid.UUID     `gorm:"type:uuid"`
	CreatedByAgent   *Agent         `gorm:"foreignKey:CreatedByAgentID;constraint:OnDelete:SET NULL"`
	CreatedByUserID  *uuid.UUID     `gorm:"type:uuid"`
	CreatedByUser    *User          `gorm:"foreignKey:CreatedByUserID;constraint:OnDelete:SET NULL"`
	ArchivedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (CanvasArtifact) TableName() string { return "canvas_artifacts" }

type CanvasArtifactFile struct {
	ID               uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID            uuid.UUID       `gorm:"type:uuid;not null;index"`
	Org              *Org            `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	CanvasArtifactID uuid.UUID       `gorm:"type:uuid;not null;index"`
	CanvasArtifact   *CanvasArtifact `gorm:"foreignKey:CanvasArtifactID;constraint:OnDelete:CASCADE"`
	Path             string          `gorm:"type:text;not null"`
	Role             string          `gorm:"type:text;not null;default:''"`
	ContentType      string          `gorm:"type:text;not null;default:''"`
	ObjectKey        string          `gorm:"type:text;not null;index"`
	SizeBytes        int64           `gorm:"not null;default:0"`
	SHA256           string          `gorm:"column:sha256;type:text;not null;default:''"`
	ArchivedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (CanvasArtifactFile) TableName() string { return "canvas_artifact_files" }
