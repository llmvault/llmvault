package model

import (
	"time"

	"github.com/google/uuid"
)

// RAGIndexAttemptError records a single per-document (or per-entity,
// per-time-range) failure that happened inside a RAGIndexAttempt.
// Multiple errors per attempt are expected — one attempt may partially
// succeed (Status=completed_with_errors) and have a fleet of these rows
// explaining which documents the admin needs to retry.
//
// Notes:
//   - PK is uuid, not autoincrement int (Hivy convention).
//   - CASCADE on the attempt FK is explicit — if the parent attempt
//     goes away (e.g. org deletion cascades through it), the error log
//     goes with it.
type RAGIndexAttemptError struct {
	// ID is the primary key.
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	// OrgID enables row-level tenancy + fast org-scoped error listings
	// in the admin UI.
	OrgID uuid.UUID `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE"`

	// IndexAttemptID — CASCADE so the error lifecycle tracks the
	// parent attempt.
	IndexAttemptID uuid.UUID       `gorm:"type:uuid;not null;index"`
	IndexAttempt   RAGIndexAttempt `gorm:"foreignKey:IndexAttemptID;constraint:OnDelete:CASCADE"`

	// RAGSourceID — mirror of what's on the parent attempt,
	// denormalised so the admin UI can filter errors by source without
	// a join.
	RAGSourceID uuid.UUID `gorm:"type:uuid;not null;index"`

	// DocumentID / DocumentLink — either or both may be null — an
	// error raised before we had a document identity (e.g. upstream
	// 500 on the list endpoint) won't have either.
	DocumentID   *string `gorm:"type:text"`
	DocumentLink *string `gorm:"type:text"`

	// EntityID / FailedTimeRangeStart / FailedTimeRangeEnd — for
	// time-range connectors (Slack, calendar) this is how we identify
	// "the slice that failed" so a subsequent retry can target just
	// that window.
	EntityID             *string `gorm:"type:text"`
	FailedTimeRangeStart *time.Time
	FailedTimeRangeEnd   *time.Time

	// FailureMessage — the human-readable error message. Non-null (an
	// error row without a message is useless).
	FailureMessage string `gorm:"type:text;not null"`

	// IsResolved — admin can mark an error as acknowledged/handled
	// without deleting the record.
	IsResolved bool `gorm:"not null;default:false"`

	// ErrorType — optional classifier (e.g. "PermissionDenied",
	// "RateLimited").
	ErrorType *string `gorm:"type:text"`

	// TimeCreated — server-defaulted so inserting apps never have to
	// supply it.
	TimeCreated time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

// TableName pins the Postgres table name.
func (RAGIndexAttemptError) TableName() string { return "rag_index_attempt_errors" }
