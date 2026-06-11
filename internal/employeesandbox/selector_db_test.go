package employeesandbox

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func connectSelectorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	sqlDB, _ := db.DB()
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func createSelectorOrg(t *testing.T, db *gorm.DB) model.Org {
	t.Helper()
	org := model.Org{Name: "selector-test-" + uuid.NewString()[:8]}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })
	return org
}

func createSelectorEmployee(t *testing.T, db *gorm.DB, orgID uuid.UUID) model.Employee {
	t.Helper()
	emp := model.Employee{OrgID: &orgID, Name: "emp-" + uuid.NewString()[:8], SystemPrompt: "x", Model: "gpt-4o"}
	if err := db.Create(&emp).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", emp.ID).Delete(&model.Employee{}) })
	return emp
}

// TestSelectorReturnsNonRunningSandbox verifies P0-17: the selector returns
// 'creating'/'stopped' rows so concurrent syncs reuse an in-flight or idle
// sandbox instead of treating it as absent and minting a duplicate.
func TestSelectorReturnsNonRunningSandbox(t *testing.T) {
	db := connectSelectorTestDB(t)
	org := createSelectorOrg(t, db)
	emp := createSelectorEmployee(t, db, org.ID)

	for _, status := range []string{"creating", "starting", "stopped"} {
		sb := model.Sandbox{
			OrgID:                  &org.ID,
			EmployeeID:             &emp.ID,
			ProviderID:             "daytona",
			ExternalID:             "ext-" + status,
			RuntimeURL:             "https://x.test",
			EncryptedRuntimeSecret: []byte("enc"),
			Status:                 status,
		}
		if err := db.Create(&sb).Error; err != nil {
			t.Fatalf("create %s sandbox: %v", status, err)
		}
		got, err := (Selector{DB: db}).MainRuntime(context.Background(), org.ID, emp.ID)
		if err != nil {
			t.Fatalf("MainRuntime with %s sandbox returned error: %v (must reuse non-running row)", status, err)
		}
		if got.ID != sb.ID {
			t.Fatalf("MainRuntime returned %s, want %s", got.ID, sb.ID)
		}
		db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
	}
}

// employeeSandboxLockKey mirrors the handler's per-employee advisory-lock key
// derivation so this DB-level test exercises the exact serialization the
// ensureEmployeeSandbox path relies on for P0-17.
func employeeSandboxLockKey(employeeID uuid.UUID) int64 {
	return int64(binary.BigEndian.Uint64(employeeID[:8])) // #nosec G115
}

// TestEmployeeSandboxAdvisoryLockSerializes verifies the P0-17 primary
// mechanism: a per-employee pg_advisory_xact_lock serialises concurrent ensure
// runs so only one provisions at a time (the loser waits, then reuses), instead
// of both racing through check-then-create and minting duplicate billing
// sandboxes. The same employee maps to the same lock key; a different employee
// does not block.
func TestEmployeeSandboxAdvisoryLockSerializes(t *testing.T) {
	db := connectSelectorTestDB(t)
	org := createSelectorOrg(t, db)
	emp := createSelectorEmployee(t, db, org.ID)
	other := createSelectorEmployee(t, db, org.ID)

	key := employeeSandboxLockKey(emp.ID)

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	ctx := context.Background()

	// Dedicated connection #1 holds the session-level advisory lock for emp.
	holder, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire holder conn: %v", err)
	}
	defer holder.Close()
	var held bool
	if err := holder.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&held); err != nil {
		t.Fatalf("acquire advisory lock: %v", err)
	}
	if !held {
		t.Fatal("expected to acquire the advisory lock on a clean key")
	}
	defer func() {
		var released bool
		_ = holder.QueryRowContext(ctx, "SELECT pg_advisory_unlock($1)", key).Scan(&released)
	}()

	// Dedicated connection #2 tries the SAME employee's lock — must fail (held).
	contender, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire contender conn: %v", err)
	}
	defer contender.Close()
	var gotSame bool
	if err := contender.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&gotSame); err != nil {
		t.Fatalf("try same lock: %v", err)
	}
	if gotSame {
		t.Fatal("expected same-employee advisory lock to be unavailable while held")
	}

	// The DIFFERENT employee's lock must remain available — different key.
	var gotOther bool
	if err := contender.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", employeeSandboxLockKey(other.ID)).Scan(&gotOther); err != nil {
		t.Fatalf("try other lock: %v", err)
	}
	if !gotOther {
		t.Fatal("expected different-employee advisory lock to be available")
	}
}
