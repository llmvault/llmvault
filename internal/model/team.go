package model

import (
	"time"

	"github.com/google/uuid"
)

type Team struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID       uuid.UUID  `gorm:"type:uuid;not null;index;uniqueIndex:idx_teams_org_name_active,priority:1,where:archived_at IS NULL"`
	Org         Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	Name        string     `gorm:"type:text;not null;uniqueIndex:idx_teams_org_name_active,priority:2,where:archived_at IS NULL"`
	Description string     `gorm:"type:text;not null;default:''"`
	CreatedBy   *uuid.UUID `gorm:"type:uuid"`
	Creator     *User      `gorm:"foreignKey:CreatedBy;constraint:OnDelete:SET NULL"`
	ArchivedAt  *time.Time `gorm:"index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Team) TableName() string { return "teams" }

type TeamMember struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID     uuid.UUID `gorm:"type:uuid;not null;index:idx_team_members_org_user,priority:1"`
	Org       Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	TeamID    uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_team_members_team_user,priority:1"`
	Team      Team      `gorm:"foreignKey:TeamID;constraint:OnDelete:CASCADE"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index:idx_team_members_org_user,priority:2;uniqueIndex:idx_team_members_team_user,priority:2"`
	User      User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	Role      string    `gorm:"type:text;not null;default:'member'"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (TeamMember) TableName() string { return "team_members" }

type OrgInviteTeam struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID       uuid.UUID `gorm:"type:uuid;not null;index"`
	Org         Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	OrgInviteID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_org_invite_teams_invite_team,priority:1"`
	OrgInvite   OrgInvite `gorm:"foreignKey:OrgInviteID;constraint:OnDelete:CASCADE"`
	TeamID      uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_org_invite_teams_invite_team,priority:2"`
	Team        Team      `gorm:"foreignKey:TeamID;constraint:OnDelete:CASCADE"`
	CreatedAt   time.Time
}

func (OrgInviteTeam) TableName() string { return "org_invite_teams" }
