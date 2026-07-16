package billing

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

// ErrInsufficientCredits is returned when Spend would drive the balance below
// zero. Handlers should translate this into HTTP 402 Payment Required.
var ErrInsufficientCredits = errors.New("billing: insufficient credits")

// ErrAlreadyRecorded is returned when a non-empty
// (org_id, reason, ref_type, ref_id) tuple has already been applied. Callers
// should treat this as success, not an error; it means the caller retried
// after the first attempt already committed.
var ErrAlreadyRecorded = errors.New("billing: spend already recorded (idempotent replay)")

// Grant reasons (stored in credit_ledger_entries.reason).
const (
	ReasonTopup        = "topup"
	ReasonAdjustment   = "adjustment"
	ReasonRefund       = "refund"
	ReasonAgentRun     = "agent_run"
	ReasonLLMTokens    = "llm_tokens"
	ReasonWelcomeGrant = "welcome_grant"
)

// RefType for welcome grants: ref_id is the new user's ID. Deployments should
// enforce idempotency for this tuple at the database layer.
const RefTypeSignup = "signup"

// CreditsService is the append-only credit ledger. Grants are positive, spends
// are negative, balance = SUM(amount).
type CreditsService struct {
	db *gorm.DB
}

// NewCreditsService creates a credit ledger service bound to db.
func NewCreditsService(db *gorm.DB) *CreditsService {
	return &CreditsService{db: db}
}

// Balance returns the org's current credit balance.
func (s *CreditsService) Balance(orgID uuid.UUID) (int64, error) {
	return sumBalance(s.db, orgID)
}

// Grant permanently adds credits to the org. amount must be positive.
func (s *CreditsService) Grant(orgID uuid.UUID, amount int64, reason, refType, refID string) error {
	return GrantWithTx(s.db, orgID, amount, reason, refType, refID)
}

// GrantWithTx writes a grant entry on the supplied DB handle, which is
// expected to be a *gorm.DB tied to an open transaction. Use this when the
// grant must be atomic with surrounding work (e.g. signup creating the user
// + org + welcome grant in one transaction).
func GrantWithTx(tx *gorm.DB, orgID uuid.UUID, amount int64, reason, refType, refID string) error {
	if amount <= 0 {
		return fmt.Errorf("grant amount must be positive (got %d)", amount)
	}
	if alreadyRecorded, err := ledgerEntryExists(tx, orgID, reason, refType, refID); err != nil {
		return err
	} else if alreadyRecorded {
		return ErrAlreadyRecorded
	}
	// The check above only narrows the race; the partial unique index is the real
	// arbiter. ON CONFLICT DO NOTHING lets the loser no-op without poisoning the
	// surrounding tx (a bare 23505 would abort the caller's renewal tx).
	res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.CreditLedgerEntry{
		OrgID:   orgID,
		Amount:  amount,
		Reason:  reason,
		RefType: refType,
		RefID:   refID,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrAlreadyRecorded
	}
	return nil
}

// Spend deducts credits. amount must be positive.
//
// Returns ErrInsufficientCredits if the org's balance would drop below zero,
// and ErrAlreadyRecorded when a deduction with the same non-empty
// (org_id, reason, ref_type, ref_id) has already been written. Async task
// retries hit this and should treat it as success.
//
// Spend serialises concurrent spends for a given org by taking a row-level
// lock on the org record inside a transaction. This trades throughput per-org
// for correctness: we never oversubscribe the balance.
func (s *CreditsService) Spend(orgID uuid.UUID, amount int64, reason, refType, refID string) error {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		return SpendWithTx(tx, orgID, amount, reason, refType, refID)
	})
	if isUniqueViolation(err) {
		return ErrAlreadyRecorded
	}
	return err
}

func SpendWithTx(tx *gorm.DB, orgID uuid.UUID, amount int64, reason, refType, refID string) error {
	if amount <= 0 {
		return fmt.Errorf("spend amount must be positive (got %d)", amount)
	}

	var org model.Org
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", orgID).First(&org).Error; err != nil {
		return fmt.Errorf("lock org: %w", err)
	}

	current, err := sumBalance(tx, orgID)
	if err != nil {
		return err
	}
	if current < amount {
		return ErrInsufficientCredits
	}
	if alreadyRecorded, err := ledgerEntryExists(tx, orgID, reason, refType, refID); err != nil {
		return err
	} else if alreadyRecorded {
		return ErrAlreadyRecorded
	}

	// ON CONFLICT DO NOTHING arbitrates concurrent spends with the same
	// reference via the partial unique index without aborting the transaction.
	res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.CreditLedgerEntry{
		OrgID:   orgID,
		Amount:  -amount,
		Reason:  reason,
		RefType: refType,
		RefID:   refID,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrAlreadyRecorded
	}
	return nil
}

func IsUniqueViolation(err error) bool { return isUniqueViolation(err) }

// isUniqueViolation reports whether err is a Postgres unique_violation
// (SQLSTATE 23505). pgx wraps these as *pgconn.PgError, so we match the
// structured Code, not the message text (which breaks on localised messages).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	// 23505 is the SQLSTATE for unique_violation.
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func sumBalance(db *gorm.DB, orgID uuid.UUID) (int64, error) {
	var row struct{ Total *int64 }
	if err := db.Model(&model.CreditLedgerEntry{}).
		Select("COALESCE(SUM(amount), 0) AS total").
		Where("org_id = ?", orgID).
		Scan(&row).Error; err != nil {
		return 0, fmt.Errorf("compute balance: %w", err)
	}
	if row.Total == nil {
		return 0, nil
	}
	return *row.Total, nil
}
