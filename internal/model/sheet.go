package model

import (
	"time"

	"github.com/google/uuid"
)

type Sheet struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID            uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org              *Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	TeamID           uuid.UUID  `gorm:"type:uuid;not null;index"`
	Team             *Team      `gorm:"foreignKey:TeamID;constraint:OnDelete:RESTRICT"`
	Slug             string     `gorm:"type:text;not null"`
	Name             string     `gorm:"type:text;not null"`
	Description      string     `gorm:"type:text;not null;default:''"`
	Icon             string     `gorm:"type:text;not null;default:''"`
	CreatedByAgentID *uuid.UUID `gorm:"type:uuid"`
	CreatedByAgent   *Agent     `gorm:"foreignKey:CreatedByAgentID;constraint:OnDelete:SET NULL"`
	CreatedByUserID  *uuid.UUID `gorm:"type:uuid"`
	CreatedByUser    *User      `gorm:"foreignKey:CreatedByUserID;constraint:OnDelete:SET NULL"`
	SourceSessionID  *uuid.UUID `gorm:"type:uuid"`
	SourceSession    *Session   `gorm:"foreignKey:SourceSessionID;constraint:OnDelete:SET NULL"`
	ArchivedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (Sheet) TableName() string { return "sheets" }

type SheetPage struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	SheetID        uuid.UUID `gorm:"type:uuid;not null;index"`
	Sheet          *Sheet    `gorm:"foreignKey:SheetID;constraint:OnDelete:CASCADE"`
	OrgID          uuid.UUID `gorm:"type:uuid;not null;index"`
	Org            *Org      `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	Name           string    `gorm:"type:text;not null"`
	Position       float64   `gorm:"type:double precision;not null"`
	DisplayFieldID *string   `gorm:"type:text"`
	ArchivedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (SheetPage) TableName() string { return "sheet_pages" }

type SheetField struct {
	ID         string     `gorm:"type:text;primaryKey"`
	PageID     uuid.UUID  `gorm:"type:uuid;not null;index"`
	Page       *SheetPage `gorm:"foreignKey:PageID;constraint:OnDelete:CASCADE"`
	OrgID      uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org        *Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	Name       string     `gorm:"type:text;not null"`
	Type       string     `gorm:"type:text;not null"`
	Options    JSON       `gorm:"type:jsonb;not null;default:'{}'"`
	Position   float64    `gorm:"type:double precision;not null"`
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (SheetField) TableName() string { return "sheet_fields" }

type SheetView struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	PageID     uuid.UUID  `gorm:"type:uuid;not null;index"`
	Page       *SheetPage `gorm:"foreignKey:PageID;constraint:OnDelete:CASCADE"`
	OrgID      uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org        *Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	Name       string     `gorm:"type:text;not null"`
	Type       string     `gorm:"type:text;not null;default:'grid'"`
	Config     JSON       `gorm:"type:jsonb;not null;default:'{}'"`
	Position   float64    `gorm:"type:double precision;not null"`
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (SheetView) TableName() string { return "sheet_views" }
