package apps

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/crypto/hkdf"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

// Injected env contract — must match what the template's hivycore.LoadConfig
// reads (global/apps/template/hivycore/config.go).
const (
	envAppID         = "HIVY_APP_ID"
	envAppSecret     = "HIVY_APP_SECRET"
	envAppAPIURL     = "HIVY_APP_API_URL"
	envAuthPublicKey = "HIVY_AUTH_PUBLIC_KEY"
	envLaunchURL     = "HIVY_LAUNCH_URL"
	envSessionSecret = "HIVY_SESSION_SECRET"
	envSheetID       = "HIVY_SHEET_ID"
)

// sessionSecretHKDFLabel is the fixed HKDF-SHA256 info label for deriving
// HIVY_SESSION_SECRET from the app secret. The derivation is deterministic on
// purpose: redeploys and env re-pushes yield the same session key, so live
// user sessions survive them. Rotating the app secret rotates the session
// key (and thus invalidates sessions) — the desired coupling.
const sessionSecretHKDFLabel = "hivy-app-session-secret-v1"

// DeriveSessionSecret derives the app's session-cookie secret from its
// runtime secret via HKDF-SHA256 with a fixed label (64 hex chars).
func DeriveSessionSecret(appSecret string) (string, error) {
	reader := hkdf.New(sha256.New, []byte(appSecret), nil, []byte(sessionSecretHKDFLabel))
	out := make([]byte, 32)
	if _, err := io.ReadFull(reader, out); err != nil {
		return "", fmt.Errorf("derive session secret: %w", err)
	}
	return hex.EncodeToString(out), nil
}

// EncodeAuthPublicKeyPEM renders the auth RSA public key as PKIX PEM with
// newlines escaped as literal `\n`: hivy-appd's env file rejects raw
// newlines, and hivycore.parseRSAPublicKey un-escapes the sequence.
func EncodeAuthPublicKeyPEM(pub *rsa.PublicKey) (string, error) {
	if pub == nil {
		return "", fmt.Errorf("auth public key is required")
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", fmt.Errorf("marshal auth public key: %w", err)
	}
	block := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	return strings.ReplaceAll(strings.TrimSpace(string(block)), "\n", `\n`), nil
}

// buildAppEnv assembles the full environment for the app process: the
// channel's user env vars first (decrypted, PLAIN names — app sandboxes have
// no Rust runtime stripping an __ENV__ prefix), then the reserved HIVY_ keys,
// which always win (channel env var names may not start with HIVY_ anyway).
func (s *Service) buildAppEnv(ctx context.Context, app *model.App, secret string) (map[string]string, error) {
	env := map[string]string{}

	var vars []model.ChannelEnvVar
	if err := s.db.WithContext(ctx).
		Where("channel_id = ?", app.ChannelID).
		Find(&vars).Error; err != nil {
		return nil, fmt.Errorf("load channel env vars: %w", err)
	}
	for _, v := range vars {
		value, err := s.encKey.DecryptString(v.EncryptedValue)
		if err != nil {
			return nil, fmt.Errorf("decrypt channel env var %q: %w", v.Name, err)
		}
		env[v.Name] = value
	}

	sessionSecret, err := DeriveSessionSecret(secret)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(s.authPublicKeyPEM) == "" {
		return nil, fmt.Errorf("apps: auth public key PEM is not configured")
	}

	env[envAppID] = app.ID.String()
	env[envAppSecret] = secret
	env[envAppAPIURL] = internalAppAPIURL(s.cfg, app.ID.String())
	env[envAuthPublicKey] = s.authPublicKeyPEM
	env[envLaunchURL] = launchURL(s.cfg, app.ID.String())
	env[envSessionSecret] = sessionSecret
	env[envSheetID] = app.SheetID.String()
	return env, nil
}

// BuildPreviewEnv assembles the exact env the deploy path injects into an app
// sandbox (buildAppEnv: channel env vars + reserved HIVY_ keys) plus PORT, for
// a preview run of the app inside the BUILDER agent's own sandbox. Every value
// is secret material — callers must never log or echo it; the values may only
// travel in the preview-env HTTP response body.
func (s *Service) BuildPreviewEnv(ctx context.Context, app *model.App, port int) (map[string]string, error) {
	secret, err := s.decryptAppSecret(app)
	if err != nil {
		return nil, err
	}
	env, err := s.buildAppEnv(ctx, app, secret)
	if err != nil {
		return nil, err
	}
	env["PORT"] = strconv.Itoa(port)
	return env, nil
}

// internalAppAPIURL is the base URL the app backend uses to reach the
// internal app API ({RuntimeControlPlaneBaseURL}/internal/apps/{appID}) —
// same config source as HIVY_DRIVE_UPLOAD_URL.
func internalAppAPIURL(cfg *config.Config, appID string) string {
	base := strings.TrimRight(cfg.RuntimeControlPlaneBaseURL(), "/")
	return base + "/internal/apps/" + appID
}

// launchURL is where the app's built-in "not signed in" page links to, and the
// origin the template's frame-ancestors CSP derives from. Apps render inside
// the session workspace's right panel (like other artifacts), so this points
// at the frontend root — there is no standalone per-app launch page.
func launchURL(cfg *config.Config, _ string) string {
	frontend := ""
	if cfg != nil {
		frontend = strings.TrimRight(strings.TrimSpace(cfg.FrontendURL), "/")
	}
	return frontend + "/"
}
