package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

const (
	defaultAppdPort = 7080
	defaultAppPort  = 8080
	defaultAppRoot  = "/workspace/app"
	defaultLogsDir  = "/workspace/logs"

	appUnitName = "hivy-app.service"

	// maxBundleBytes caps the compressed bundle download size.
	maxBundleBytes = 512 << 20
	// maxZipEntries caps the number of entries extracted from a bundle.
	maxZipEntries = 10000
	// maxUnpackedBytes caps the total uncompressed size of a bundle.
	maxUnpackedBytes = 1 << 30
	// maxLogLines is the hard cap on lines returned by /logs after filtering.
	maxLogLines = 2000
	// defaultLogLines is used when the lines query param is absent.
	defaultLogLines = 200
	// rotateAtBytes is the size threshold that triggers log rotation.
	rotateAtBytes = 10 << 20
	// rotateKeep is how many rotated generations to keep (file.1 .. file.N).
	rotateKeep = 3
)

type config struct {
	// Secret is the app secret used as the bearer token on every endpoint.
	Secret string
	// Port is the appd listen port.
	Port int
	// AppRoot holds releases/, the current symlink, the env file, and tmp/.
	AppRoot string
	// LogsDir holds app.log and appd.log (plus rotated generations).
	LogsDir string
	// AppPort is the app's local HTTP port, probed by /health.
	AppPort int
}

func loadConfig() (config, error) {
	cfg := config{
		Secret:  os.Getenv("HIVY_APP_SECRET"),
		Port:    defaultAppdPort,
		AppRoot: envOr("APP_ROOT", defaultAppRoot),
		LogsDir: envOr("LOGS_DIR", defaultLogsDir),
		AppPort: defaultAppPort,
	}
	if cfg.Secret == "" {
		return cfg, errors.New("HIVY_APP_SECRET is required")
	}
	port, err := envPort("HIVY_APPD_PORT", defaultAppdPort)
	if err != nil {
		return cfg, err
	}
	cfg.Port = port
	appPort, err := envPort("HIVY_APP_PORT", defaultAppPort)
	if err != nil {
		return cfg, err
	}
	cfg.AppPort = appPort
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envPort(key string, fallback int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid %s: %q", key, raw)
	}
	return port, nil
}

func (c config) releasesDir() string { return c.AppRoot + "/releases" }
func (c config) tmpDir() string      { return c.AppRoot + "/tmp" }
func (c config) currentLink() string { return c.AppRoot + "/current" }
func (c config) envFile() string     { return c.AppRoot + "/env" }
func (c config) appLogPath() string  { return c.LogsDir + "/app.log" }
func (c config) appdLogPath() string { return c.LogsDir + "/appd.log" }
