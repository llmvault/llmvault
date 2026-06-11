package employeeruntime

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// TestAttachProxyTokenToSandbox verifies P2-46: the attach binds the exact
// minted token to the sandbox, refuses revoked/expired/unknown tokens, and
// surfaces lookup errors instead of swallowing them — so the refresh scheduler
// never computes its schedule from the wrong row.
func TestAttachProxyTokenToSandbox_TagsMintedTokenOnly(t *testing.T) {
	db := connectCompileTestDB(t)
	agent := createCompileTokenAgent(t, db)
	deps := CompileDeps{DB: db, SigningKey: []byte("test-signing-key-32-bytes-long!!")}
	sandboxID := uuid.New()

	// An older token for the same employee/sandbox that must NOT be re-tagged
	// just because it is the most recent matching row.
	stale, err := MintProxyToken(context.Background(), deps, &agent, uuid.Nil)
	if err != nil {
		t.Fatalf("mint stale token: %v", err)
	}
	// The token actually minted for this sandbox launch.
	minted, err := MintProxyToken(context.Background(), deps, &agent, uuid.Nil)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}

	if err := AttachProxyTokenToSandbox(context.Background(), deps, &agent, sandboxID, minted.JTI); err != nil {
		t.Fatalf("attach minted token: %v", err)
	}

	var got model.Token
	if err := db.First(&got, "jti = ?", minted.JTI).Error; err != nil {
		t.Fatalf("load minted token: %v", err)
	}
	if got.Meta[model.TokenMetaSandboxID] != sandboxID.String() {
		t.Fatalf("minted token sandbox_id = %#v, want %s", got.Meta[model.TokenMetaSandboxID], sandboxID)
	}
	var staleTok model.Token
	if err := db.First(&staleTok, "jti = ?", stale.JTI).Error; err != nil {
		t.Fatalf("load stale token: %v", err)
	}
	if staleTok.Meta[model.TokenMetaSandboxID] == sandboxID.String() {
		t.Fatal("stale token was wrongly tagged with the sandbox id")
	}
}

func TestAttachProxyTokenToSandbox_RejectsRevokedToken(t *testing.T) {
	db := connectCompileTestDB(t)
	agent := createCompileTokenAgent(t, db)
	deps := CompileDeps{DB: db, SigningKey: []byte("test-signing-key-32-bytes-long!!")}

	minted, err := MintProxyToken(context.Background(), deps, &agent, uuid.Nil)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	now := time.Now().UTC()
	if err := db.Model(&model.Token{}).Where("jti = ?", minted.JTI).Update("revoked_at", now).Error; err != nil {
		t.Fatalf("revoke token: %v", err)
	}

	err = AttachProxyTokenToSandbox(context.Background(), deps, &agent, uuid.New(), minted.JTI)
	if err == nil {
		t.Fatal("attach must fail for a revoked token")
	}
}

func TestAttachProxyTokenToSandbox_RejectsExpiredToken(t *testing.T) {
	db := connectCompileTestDB(t)
	agent := createCompileTokenAgent(t, db)
	deps := CompileDeps{DB: db, SigningKey: []byte("test-signing-key-32-bytes-long!!")}

	minted, err := MintProxyToken(context.Background(), deps, &agent, uuid.Nil)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	if err := db.Model(&model.Token{}).Where("jti = ?", minted.JTI).
		Update("expires_at", time.Now().UTC().Add(-time.Hour)).Error; err != nil {
		t.Fatalf("expire token: %v", err)
	}

	err = AttachProxyTokenToSandbox(context.Background(), deps, &agent, uuid.New(), minted.JTI)
	if err == nil {
		t.Fatal("attach must fail for an expired token")
	}
}

func TestAttachProxyTokenToSandbox_RejectsUnknownAndEmptyJTI(t *testing.T) {
	db := connectCompileTestDB(t)
	agent := createCompileTokenAgent(t, db)
	deps := CompileDeps{DB: db, SigningKey: []byte("test-signing-key-32-bytes-long!!")}

	if err := AttachProxyTokenToSandbox(context.Background(), deps, &agent, uuid.New(), "does-not-exist"); err == nil {
		t.Fatal("attach must fail for an unknown jti")
	}
	if err := AttachProxyTokenToSandbox(context.Background(), deps, &agent, uuid.New(), ""); err == nil {
		t.Fatal("attach must fail for an empty jti")
	}
}
