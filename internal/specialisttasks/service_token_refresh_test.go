package specialisttasks

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/model"
)

func specialistTokenTestEncKey(t *testing.T) *crypto.SymmetricKey {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 7)
	}
	sk, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("enc key: %v", err)
	}
	return sk
}

// seedSpecialistRefreshScope creates an org/employee/credential/sandbox and a
// Service wired with the DB + compileDeps needed to mint specialist tokens.
func seedSpecialistRefreshScope(t *testing.T) (*Service, employeeruntime.CompileDeps, model.Org, model.Employee, model.Sandbox) {
	t.Helper()
	db := connectSpecialistTasksTestDB(t)
	encKey := specialistTokenTestEncKey(t)

	org := model.Org{ID: uuid.New(), Name: "specialist-refresh-" + uuid.NewString()[:8]}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	cred := model.Credential{
		ID: uuid.New(), OrgID: &org.ID, Label: "spec-refresh",
		BaseURL: "https://proxy.test", AuthScheme: "bearer",
		EncryptedKey: []byte("enc"), WrappedDEK: []byte("dek"), ProviderID: "openrouter",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	employee := model.Employee{
		ID: uuid.New(), OrgID: &org.ID, Name: "emp-" + uuid.NewString()[:8],
		IsEmployee: true, Harness: "employee-sandbox", Model: "test-model",
		CredentialID: &cred.ID, Status: "active", SystemPrompt: "x",
		AttachedSpecialists: pq.StringArray{"software-engineering-specialist"},
	}
	if err := db.Create(&employee).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	sb := model.Sandbox{
		ID: uuid.New(), OrgID: &org.ID, EmployeeID: &employee.ID,
		ExternalID: "spec-refresh-ext", RuntimeURL: "https://stub.example",
		EncryptedRuntimeSecret: []byte("enc"), Status: "running",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Token{})
		db.Where("org_id = ?", org.ID).Delete(&model.Sandbox{})
		db.Where("org_id = ?", org.ID).Delete(&model.Employee{})
		db.Where("org_id = ?", org.ID).Delete(&model.Credential{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	compileDeps := employeeruntime.CompileDeps{
		DB: db, EncKey: encKey,
		SigningKey: []byte("test-signing-key-32-bytes-long!!"),
	}
	svc := &Service{
		db:          db,
		catalog:     specialistTestCatalog(t),
		compileDeps: compileDeps,
		now:         time.Now,
	}
	return svc, compileDeps, org, employee, sb
}

// TestLatestSpecialistToken verifies the helper selects the newest live
// specialist token bound to the sandbox+slug, ignoring revoked ones.
func TestLatestSpecialistToken(t *testing.T) {
	svc, deps, org, employee, sb := seedSpecialistRefreshScope(t)
	slug := "software-engineering-specialist"
	ctx := context.Background()

	if _, err := svc.latestSpecialistToken(ctx, org.ID, employee.ID, sb.ID, slug); err != nil {
		t.Fatalf("latest with no tokens: %v", err)
	}
	first, err := svc.latestSpecialistToken(ctx, org.ID, employee.ID, sb.ID, slug)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if first != nil {
		t.Fatal("expected nil when no specialist token exists")
	}

	minted, err := employeeruntime.MintSpecialistProxyToken(ctx, deps, &employee, sb.ID, slug)
	if err != nil {
		t.Fatalf("mint specialist token: %v", err)
	}
	got, err := svc.latestSpecialistToken(ctx, org.ID, employee.ID, sb.ID, slug)
	if err != nil {
		t.Fatalf("latest after mint: %v", err)
	}
	if got == nil || got.JTI != minted.JTI {
		t.Fatalf("latest token = %#v, want jti %s", got, minted.JTI)
	}
}

// TestRevokeOlderSpecialistTokens verifies the P1-18/P1-19 revoke logic for
// specialists: the kept token and any token minted within the grace window
// survive; older tokens are revoked.
func TestRevokeOlderSpecialistTokens(t *testing.T) {
	svc, deps, org, employee, sb := seedSpecialistRefreshScope(t)
	slug := "software-engineering-specialist"
	ctx := context.Background()

	oldTok, err := employeeruntime.MintSpecialistProxyToken(ctx, deps, &employee, sb.ID, slug)
	if err != nil {
		t.Fatalf("mint old: %v", err)
	}
	if err := svc.db.Model(&model.Token{}).Where("jti = ?", oldTok.JTI).
		Update("created_at", time.Now().Add(-2*specialistProxyTokenRevokeGrace)).Error; err != nil {
		t.Fatalf("backdate old: %v", err)
	}
	concurrentTok, err := employeeruntime.MintSpecialistProxyToken(ctx, deps, &employee, sb.ID, slug)
	if err != nil {
		t.Fatalf("mint concurrent: %v", err)
	}
	keepTok, err := employeeruntime.MintSpecialistProxyToken(ctx, deps, &employee, sb.ID, slug)
	if err != nil {
		t.Fatalf("mint keep: %v", err)
	}

	if err := svc.revokeOlderSpecialistTokens(ctx, org.ID, employee.ID, sb.ID, slug, keepTok.JTI); err != nil {
		t.Fatalf("revoke older: %v", err)
	}

	assertRevoked := func(jti string, want bool) {
		var tok model.Token
		if err := svc.db.First(&tok, "jti = ?", jti).Error; err != nil {
			t.Fatalf("load %s: %v", jti, err)
		}
		if (tok.RevokedAt != nil) != want {
			t.Fatalf("token %s revoked=%v, want %v", jti, tok.RevokedAt != nil, want)
		}
	}
	assertRevoked(oldTok.JTI, true)         // older than grace -> revoked
	assertRevoked(concurrentTok.JTI, false) // within grace -> kept (P1-19)
	assertRevoked(keepTok.JTI, false)       // the kept token -> kept
}
