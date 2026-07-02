package model

import (
	"time"

	"github.com/google/uuid"
)

type SheetRow struct {
	ID               uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	PageID           uuid.UUID       `gorm:"type:uuid;not null;index"`
	Page             *SheetPage      `gorm:"foreignKey:PageID;constraint:OnDelete:CASCADE"`
	OrgID            uuid.UUID       `gorm:"type:uuid;not null;index"`
	Org              *Org            `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	Data             JSON            `gorm:"type:jsonb;not null;default:'{}'"`
	Position         float64         `gorm:"type:double precision;not null"`
	ImportJobID      *uuid.UUID      `gorm:"type:uuid"`
	ImportJob        *SheetImportJob `gorm:"foreignKey:ImportJobID;constraint:OnDelete:SET NULL"`
	CreatedByAgentID *uuid.UUID      `gorm:"type:uuid"`
	CreatedByAgent   *Agent          `gorm:"foreignKey:CreatedByAgentID;constraint:OnDelete:SET NULL"`
	CreatedByUserID  *uuid.UUID      `gorm:"type:uuid"`
	CreatedByUser    *User           `gorm:"foreignKey:CreatedByUserID;constraint:OnDelete:SET NULL"`
	ArchivedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (SheetRow) TableName() string { return "sheet_rows" }

type SheetImportJob struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID            uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org              *Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	PageID           uuid.UUID  `gorm:"type:uuid;not null;index"`
	Page             *SheetPage `gorm:"foreignKey:PageID;constraint:OnDelete:CASCADE"`
	ObjectKey        string     `gorm:"type:text;not null"`
	Status           string     `gorm:"type:text;not null;default:'pending'"`
	TotalRows        int64      `gorm:"not null;default:0"`
	ProcessedRows    int64      `gorm:"not null;default:0"`
	Error            string     `gorm:"type:text;not null;default:''"`
	Options          JSON       `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedByAgentID *uuid.UUID `gorm:"type:uuid"`
	CreatedByAgent   *Agent     `gorm:"foreignKey:CreatedByAgentID;constraint:OnDelete:SET NULL"`
	CreatedByUserID  *uuid.UUID `gorm:"type:uuid"`
	CreatedByUser    *User      `gorm:"foreignKey:CreatedByUserID;constraint:OnDelete:SET NULL"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (SheetImportJob) TableName() string { return "sheet_import_jobs" }

type SheetOperation struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID             uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org               *Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	PageID            uuid.UUID  `gorm:"type:uuid;not null;index"`
	Page              *SheetPage `gorm:"foreignKey:PageID;constraint:OnDelete:CASCADE"`
	Type              string     `gorm:"type:text;not null"`
	RowCount          int        `gorm:"not null;default:0"`
	Inverse           JSON       `gorm:"type:jsonb;not null;default:'{}'"`
	ActorAgentID      *uuid.UUID `gorm:"type:uuid"`
	ActorAgent        *Agent     `gorm:"foreignKey:ActorAgentID;constraint:OnDelete:SET NULL"`
	ActorUserID       *uuid.UUID `gorm:"type:uuid"`
	ActorUser         *User      `gorm:"foreignKey:ActorUserID;constraint:OnDelete:SET NULL"`
	SourceSessionID   *uuid.UUID `gorm:"type:uuid"`
	SourceSession     *Session   `gorm:"foreignKey:SourceSessionID;constraint:OnDelete:SET NULL"`
	RevertedAt        *time.Time
	RevertedByAgentID *uuid.UUID `gorm:"type:uuid"`
	RevertedByAgent   *Agent     `gorm:"foreignKey:RevertedByAgentID;constraint:OnDelete:SET NULL"`
	RevertedByUserID  *uuid.UUID `gorm:"type:uuid"`
	RevertedByUser    *User      `gorm:"foreignKey:RevertedByUserID;constraint:OnDelete:SET NULL"`
	CreatedAt         time.Time
}

func (SheetOperation) TableName() string { return "sheet_operations" }
