package model

import (
	"time"

	"github.com/google/uuid"
)

type Brand struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID       uuid.UUID `gorm:"type:uuid;not null;index:idx_brands_org_created,priority:1;uniqueIndex:idx_brands_org_slug_active,priority:1,where:archived_at IS NULL;uniqueIndex:idx_brands_org_default_active,priority:1,where:is_default AND archived_at IS NULL"`
	Org         Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	Name        string    `gorm:"type:text;not null"`
	Slug        string    `gorm:"type:text;not null;uniqueIndex:idx_brands_org_slug_active,priority:2,where:archived_at IS NULL"`
	Description string    `gorm:"type:text;not null;default:''"`
	IsDefault   bool      `gorm:"not null;default:false;uniqueIndex:idx_brands_org_default_active,priority:2,where:is_default AND archived_at IS NULL"`
	Logos       RawJSON   `gorm:"type:jsonb;not null;default:'{}'"`
	Colors      RawJSON   `gorm:"type:jsonb;not null;default:'{}'"`
	Typography  RawJSON   `gorm:"type:jsonb;not null;default:'{}'"`
	Voice       RawJSON   `gorm:"type:jsonb;not null;default:'{}'"`
	Source      RawJSON   `gorm:"type:jsonb;not null;default:'{\"version\":1,\"origin\":\"manual\"}'"`
	RawImport   *RawJSON  `gorm:"type:jsonb"`
	ArchivedAt  *time.Time
	CreatedBy   *uuid.UUID `gorm:"type:uuid"`
	Creator     *User      `gorm:"foreignKey:CreatedBy;constraint:OnDelete:SET NULL"`
	CreatedAt   time.Time  `gorm:"index:idx_brands_org_created,priority:2,sort:desc"`
	UpdatedAt   time.Time
}

func (Brand) TableName() string { return "brands" }

type BrandAsset struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID       uuid.UUID `gorm:"type:uuid;not null;index:idx_brand_assets_org_brand,priority:1"`
	Org         Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	BrandID     uuid.UUID `gorm:"type:uuid;not null;index:idx_brand_assets_org_brand,priority:2"`
	Brand       Brand     `gorm:"foreignKey:BrandID;constraint:OnDelete:CASCADE"`
	Kind        string    `gorm:"type:text;not null;index"`
	Role        string    `gorm:"type:text;not null;default:''"`
	Name        string    `gorm:"type:text;not null"`
	Key         string    `gorm:"type:text;not null;uniqueIndex"`
	PublicURL   string    `gorm:"type:text;not null"`
	ContentType string    `gorm:"type:text;not null"`
	Bytes       int64     `gorm:"not null"`
	Width       *int
	Height      *int
	Metadata    JSON `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedBy   *uuid.UUID
	Creator     *User `gorm:"foreignKey:CreatedBy;constraint:OnDelete:SET NULL"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (BrandAsset) TableName() string { return "brand_assets" }
