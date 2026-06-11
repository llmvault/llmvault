package handler_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/email"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// A self-service email change must never be auto-confirmed, even when
// HIVY_AUTO_CONFIRM_EMAIL is set: otherwise a user could change to an
// allowlisted platform-admin address and gain platform admin.
func TestIntegration_UpdateProfile_EmailChangeRequiresReverification_WithAutoConfirm(t *testing.T) {
	db := connectTestDB(t)

	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	authHandler := handler.NewAuthHandler(
		db, pk, []byte("test-signing-key-for-refresh-tokens"),
		"hivy-test", "http://localhost:8080",
		15*time.Minute, 720*time.Hour,
		&email.LogSender{},
		"http://localhost:3000",
		true, // autoConfirmEmail — the dangerous configuration
		billing.NewCreditsService(db),
	)

	now := time.Now().UTC()
	original := "p14-original-" + uuid.NewString() + "@example.com"
	user := model.User{Email: original, Name: "Email Change User", EmailConfirmedAt: &now}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", user.ID).Delete(&model.User{}) })

	newEmail := "p14-admin-" + uuid.NewString() + "@example.com"
	body, _ := json.Marshal(map[string]any{"email": newEmail})
	req := httptest.NewRequest("PATCH", "/auth/me", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{UserID: user.ID.String()})

	rr := httptest.NewRecorder()
	authHandler.UpdateProfile(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var updated model.User
	if err := db.First(&updated, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if updated.Email != newEmail {
		t.Fatalf("expected email %q, got %q", newEmail, updated.Email)
	}
	if updated.EmailConfirmedAt != nil {
		t.Fatal("email change must reset email_confirmed_at to require re-verification, but it remained confirmed (auto-confirm escalation path)")
	}

	var resp struct {
		Email          string `json:"email"`
		EmailConfirmed bool   `json:"email_confirmed"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.EmailConfirmed {
		t.Fatal("response should report email_confirmed=false after a self-service email change")
	}
}
