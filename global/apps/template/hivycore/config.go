// DO NOT EDIT — hivycore is the Hivy platform core. See doc.go.

package hivycore

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config is the platform-injected app configuration, read once at startup
// from the environment the deploy daemon writes for the app unit.
type Config struct {
	// AppID is this app's ID; launch tokens must carry a matching app_id.
	AppID string
	// AppSecret authenticates the backend to the internal Hivy app API.
	AppSecret string
	// APIBaseURL is the internal app API base
	// ({control plane}/internal/apps/{appID}), without a trailing slash.
	APIBaseURL string
	// AuthPublicKey verifies RS256 launch tokens minted by the platform.
	AuthPublicKey *rsa.PublicKey
	// LaunchURL is the Hivy frontend launch endpoint for this app. Nothing
	// ever redirects to it: it is linked from the "not signed in" page and
	// its origin derives the frame-ancestors embedding policy.
	LaunchURL string
	// SessionSecret derives the AES-256-GCM session-cookie key.
	SessionSecret string
	// SheetID is the one sheet this app is bound to (informational — the
	// internal API is already scoped to it server-side).
	SheetID string
	// Port is the listen port (PORT env, default 8080). Production never sets
	// PORT — it always gets the 8080 default — because the platform's
	// sandbox alias/proxy is hard-bound to port 8080 (see
	// sandboxes/app/hivy-app.service and internal/apps/image.go's appPort).
	// Changing this default (or the app.json/Makefile build commands) does
	// not change what the platform proxies to; it would just make the
	// deployed app unreachable. `make preview`'s PORT=… is a local-sandbox
	// convenience only and never applies in production.
	Port string
	// PublicDir is the static SPA directory (default "public").
	PublicDir string
	// ActivityReportURL, when set (HIVY_APP_ACTIVITY_URL), is the endpoint the
	// app pings — debounced, on served requests — so the platform knows the app
	// is being used and its idle timer resets. Empty (docker/local) disables it.
	ActivityReportURL string
}

// LoadConfig reads and validates the environment, failing fast with one
// error naming every missing variable.
func LoadConfig() (*Config, error) {
	var missing []string
	need := func(name string) string {
		v := strings.TrimSpace(os.Getenv(name))
		if v == "" {
			missing = append(missing, name)
		}
		return v
	}

	cfg := &Config{
		AppID:             need("HIVY_APP_ID"),
		AppSecret:         need("HIVY_APP_SECRET"),
		APIBaseURL:        strings.TrimRight(need("HIVY_APP_API_URL"), "/"),
		LaunchURL:         need("HIVY_LAUNCH_URL"),
		SessionSecret:     need("HIVY_SESSION_SECRET"),
		SheetID:           need("HIVY_SHEET_ID"),
		Port:              strings.TrimSpace(os.Getenv("PORT")),
		PublicDir:         "public",
		ActivityReportURL: strings.TrimRight(strings.TrimSpace(os.Getenv("HIVY_APP_ACTIVITY_URL")), "/"),
	}
	pemStr := need("HIVY_AUTH_PUBLIC_KEY")

	if len(missing) > 0 {
		return nil, fmt.Errorf("hivycore: missing required environment variables: %s", strings.Join(missing, ", "))
	}

	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if n, err := strconv.Atoi(cfg.Port); err != nil || n < 1 || n > 65535 {
		return nil, fmt.Errorf("hivycore: PORT must be a port number, got %q", cfg.Port)
	}

	key, err := parseRSAPublicKey(pemStr)
	if err != nil {
		return nil, fmt.Errorf("hivycore: HIVY_AUTH_PUBLIC_KEY: %w", err)
	}
	cfg.AuthPublicKey = key

	return cfg, nil
}

// parseRSAPublicKey decodes a PEM RSA public key in either PKIX ("PUBLIC
// KEY") or PKCS#1 ("RSA PUBLIC KEY") form. Escaped "\n" sequences are
// tolerated because env files sometimes carry PEM on one line.
func parseRSAPublicKey(pemStr string) (*rsa.PublicKey, error) {
	pemStr = strings.ReplaceAll(pemStr, `\n`, "\n")
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("value is not PEM-encoded")
	}
	if pub, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("PEM block is not an RSA public key")
		}
		return rsaPub, nil
	}
	if pub, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return pub, nil
	}
	return nil, errors.New("PEM block is not a valid RSA public key (PKIX or PKCS#1)")
}
