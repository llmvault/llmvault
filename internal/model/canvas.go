package model

import (
	"time"

	"github.com/google/uuid"
)

type CanvasProject struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID            uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org              *Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	PenpotProjectID  uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex"`
	Name             string     `gorm:"type:text;not null;default:''"`
	CreatedByAgentID *uuid.UUID `gorm:"type:uuid"`
	CreatedByAgent   *Agent     `gorm:"foreignKey:CreatedByAgentID;constraint:OnDelete:SET NULL"`
	CreatedByUserID  *uuid.UUID `gorm:"type:uuid"`
	CreatedByUser    *User      `gorm:"foreignKey:CreatedByUserID;constraint:OnDelete:SET NULL"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (CanvasProject) TableName() string { return "canvas_projects" }

type CanvasFile struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID            uuid.UUID      `gorm:"type:uuid;not null;index"`
	Org              *Org           `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	CanvasProjectID  *uuid.UUID     `gorm:"type:uuid;index"`
	CanvasProject    *CanvasProject `gorm:"foreignKey:CanvasProjectID;constraint:OnDelete:SET NULL"`
	PenpotProjectID  uuid.UUID      `gorm:"type:uuid;not null;index"`
	PenpotFileID     uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex"`
	PenpotPageID     *uuid.UUID     `gorm:"type:uuid"`
	Name             string         `gorm:"type:text;not null;default:''"`
	CreatedByAgentID *uuid.UUID     `gorm:"type:uuid"`
	CreatedByAgent   *Agent         `gorm:"foreignKey:CreatedByAgentID;constraint:OnDelete:SET NULL"`
	CreatedByUserID  *uuid.UUID     `gorm:"type:uuid"`
	CreatedByUser    *User          `gorm:"foreignKey:CreatedByUserID;constraint:OnDelete:SET NULL"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (CanvasFile) TableName() string { return "canvas_files" }
