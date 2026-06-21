package handler

import (
	"context"
	"crypto/rsa"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/email"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/goroutine"
)

type loginAttempt struct {
	failures int
	firstAt  time.Time
}

type AuthHandler struct {
	db               *gorm.DB
	privateKey       *rsa.PrivateKey
	signingKey       []byte // HMAC key for refresh tokens (JWT_SIGNING_KEY)
	issuer           string
	audience         string
	accessTTL        time.Duration
	refreshTTL       time.Duration
	emailSender      email.Sender
	frontendURL      string
	autoConfirmEmail bool
	credits          *billing.CreditsService
	agentSyncer      OrgAgentSyncer
	enqueuer         enqueue.TaskEnqueuer

	loginMu       sync.Mutex
	loginAttempts map[string]*loginAttempt // keyed by email
}

func (h *AuthHandler) SetAgentSyncer(syncer OrgAgentSyncer) {
	h.agentSyncer = syncer
}

func (h *AuthHandler) SetEnqueuer(enq enqueue.TaskEnqueuer) {
	h.enqueuer = enq
}

func NewAuthHandler(db *gorm.DB, privateKey *rsa.PrivateKey, signingKey []byte, issuer, audience string, accessTTL, refreshTTL time.Duration, emailSender email.Sender, frontendURL string, autoConfirmEmail bool, credits *billing.CreditsService) *AuthHandler {
	h := &AuthHandler{
		db:               db,
		privateKey:       privateKey,
		signingKey:       signingKey,
		issuer:           issuer,
		audience:         audience,
		accessTTL:        accessTTL,
		refreshTTL:       refreshTTL,
		emailSender:      emailSender,
		frontendURL:      frontendURL,
		autoConfirmEmail: autoConfirmEmail,
		credits:          credits,
		loginAttempts:    make(map[string]*loginAttempt),
	}

	return h
}

// StartCleanup starts a background goroutine that evicts stale login attempts
// every 5 minutes. The goroutine stops when ctx is cancelled.
func (h *AuthHandler) StartCleanup(ctx context.Context) {
	goroutine.Go(ctx, func(ctx context.Context) {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.loginMu.Lock()
				cutoff := time.Now().Add(-15 * time.Minute)
				for email, a := range h.loginAttempts {
					if a.firstAt.Before(cutoff) {
						delete(h.loginAttempts, email)
					}
				}
				h.loginMu.Unlock()
			}
		}
	})
}
