// DO NOT EDIT — hivycore is the Hivy platform core. See doc.go.

package hivycore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// SessionCookieName is the app's session cookie. It is scoped to the app's
// own origin (apps live on their own subdomain) and must survive the
// cross-origin iframe the Hivy frontend mounts the app in: HttpOnly, Secure,
// SameSite=None, Partitioned (see sessionCookie).
const SessionCookieName = "hivy_app_session"

// SessionTTL is how long an app session lives before the user is bounced
// through the (usually invisible) Hivy launch round-trip again.
const SessionTTL = 7 * 24 * time.Hour

// Session is the authenticated user attached to every request that passed
// the auth middleware. It is the decrypted session-cookie payload, populated
// from the launch token's claims.
type Session struct {
	UserID     string `json:"user_id"`
	UserName   string `json:"user_name"`
	UserEmail  string `json:"user_email"`
	UserAvatar string `json:"user_avatar"`
	OrgID      string `json:"org_id"`
	OrgName    string `json:"org_name"`
	Role       string `json:"role"`
	// ExpiresAt is the session expiry as a Unix timestamp (seconds).
	ExpiresAt int64 `json:"expires_at"`
}

// sessionCodec encrypts and decrypts session cookies with AES-256-GCM under
// a key derived from HIVY_SESSION_SECRET via HKDF-SHA256 (zero 32-byte salt,
// info "session") — the same construction as the platform web session
// (apps/web/lib/auth/session.ts). Wire format: base64(iv[12] || ciphertext+tag).
type sessionCodec struct {
	aead cipher.AEAD
}

func newSessionCodec(secret string) (*sessionCodec, error) {
	if secret == "" {
		return nil, errors.New("hivycore: session secret is empty")
	}
	key, err := hkdf.Key(sha256.New, []byte(secret), make([]byte, 32), "session", 32)
	if err != nil {
		return nil, fmt.Errorf("hivycore: deriving session key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("hivycore: session cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("hivycore: session gcm: %w", err)
	}
	return &sessionCodec{aead: aead}, nil
}

func (c *sessionCodec) encrypt(s Session) (string, error) {
	plaintext, err := json.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("hivycore: encoding session: %w", err)
	}
	iv := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("hivycore: session nonce: %w", err)
	}
	sealed := c.aead.Seal(iv, iv, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// decrypt authenticates and decodes a session cookie value. Any tampering,
// truncation, wrong-key, or expired payload returns an error — callers treat
// every error as "no session".
func (c *sessionCodec) decrypt(value string) (*Session, error) {
	buf, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, errors.New("hivycore: session cookie is not base64")
	}
	ns := c.aead.NonceSize()
	if len(buf) <= ns {
		return nil, errors.New("hivycore: session cookie too short")
	}
	plaintext, err := c.aead.Open(nil, buf[:ns], buf[ns:], nil)
	if err != nil {
		return nil, errors.New("hivycore: session cookie failed authentication")
	}
	var s Session
	if err := json.Unmarshal(plaintext, &s); err != nil {
		return nil, errors.New("hivycore: session payload is not valid JSON")
	}
	if s.ExpiresAt <= time.Now().Unix() {
		return nil, errors.New("hivycore: session expired")
	}
	return &s, nil
}

// sessionCookie builds the Set-Cookie for an encrypted session value:
// HttpOnly, Secure, SameSite=None, Partitioned, Path=/, Max-Age = SessionTTL.
//
// Apps run inside a cross-origin iframe on the Hivy frontend, so the cookie
// must be SameSite=None to be sent at all in that context, and Partitioned
// (CHIPS) so browsers that block unpartitioned third-party cookies still
// accept it — partitioned to the embedding Hivy site, which is the only
// place the app renders. Both require Secure. Local-dev caveat: browsers
// treat http://localhost and http://127.0.0.1 as trustworthy, so the Secure
// cookie still works over plain http there.
func sessionCookie(value string) *http.Cookie {
	return &http.Cookie{
		Name:        SessionCookieName,
		Value:       value,
		Path:        "/",
		MaxAge:      int(SessionTTL / time.Second),
		Expires:     time.Now().Add(SessionTTL),
		HttpOnly:    true,
		Secure:      true,
		SameSite:    http.SameSiteNoneMode,
		Partitioned: true,
	}
}
