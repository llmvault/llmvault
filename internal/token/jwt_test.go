package token

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateDetailedExpiredTokenReturnsClaimsAndExpiredReason(t *testing.T) {
	signingKey := []byte("test-signing-key-32-bytes-long")
	orgID := uuid.New().String()
	credID := uuid.New().String()
	tokenString, jti, err := Mint(signingKey, orgID, credID, -time.Minute)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}

	claims, reason, err := ValidateDetailed(signingKey, tokenString)
	if err == nil {
		t.Fatal("expected expired token error")
	}
	if reason != ValidationExpired {
		t.Fatalf("reason = %q, want %q", reason, ValidationExpired)
	}
	if claims == nil {
		t.Fatal("claims were nil")
	}
	if claims.ID != jti {
		t.Fatalf("jti = %q, want %q", claims.ID, jti)
	}
	if claims.OrgID != orgID {
		t.Fatalf("org_id = %q, want %q", claims.OrgID, orgID)
	}
	if claims.CredentialID != credID {
		t.Fatalf("cred_id = %q, want %q", claims.CredentialID, credID)
	}
}

func TestValidateDetailedWrongSignatureDoesNotReportExpired(t *testing.T) {
	tokenString, _, err := Mint([]byte("right-signing-key-32-bytes"), uuid.New().String(), uuid.New().String(), -time.Minute)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}

	claims, reason, err := ValidateDetailed([]byte("wrong-signing-key-32-bytes"), tokenString)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if reason != ValidationInvalid {
		t.Fatalf("reason = %q, want %q", reason, ValidationInvalid)
	}
	if claims != nil {
		t.Fatal("claims should not be returned for invalid signature")
	}
}
