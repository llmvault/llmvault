package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/microsandbox/api"
)

type Config struct {
	Environment               string
	LogLevel                  string
	LogFormat                 string
	Addr                      string
	DatabaseDSN               string
	SentryDSN                 string
	DatabaseMaxOpenConns      int
	DatabaseMaxIdleConns      int
	DatabaseConnMaxLifetime   time.Duration
	SchedulerPlacementTimeout time.Duration

	APIToken           string
	RunnerJoinSecret   string
	RunnerAPIToken     string
	PreviewBaseDomain  string
	PreviewJWTSecret   string
	PreviewPasswordKey string
	PreviewCacheURL    string
	PreviewCacheToken  string
	PreviewCacheSync   time.Duration

	HeartbeatInterval    time.Duration
	RunnerUnhealthyAfter time.Duration
	RunnerCheckInterval  time.Duration

	ControlURL                  string
	RunnerName                  string
	RunnerPublicURL             string
	RunnerPreviewBaseURL        string
	RunnerBackend               string
	RunnerTotalCPU              int
	RunnerTotalMemoryMB         int
	RunnerTotalDiskGB           int
	RunnerCPUOvercommit         float64
	RunnerMemoryOvercommit      float64
	RunnerDiskOvercommit        float64
	RunnerPreviewPortRangeStart int
	RunnerPreviewPortRangeEnd   int
	RunnerDNSNameserver         string
	RunnerPrivateEgressCIDR     string
	RunnerPrivateEgressPorts    string
	ImageRegistry               string
}

func Load() Config {
	return Config{
		Environment:                 get("HIVY_MICROSANDBOX_ENVIRONMENT", "development"),
		LogLevel:                    get("HIVY_MICROSANDBOX_LOG_LEVEL", "info"),
		LogFormat:                   get("HIVY_MICROSANDBOX_LOG_FORMAT", "json"),
		Addr:                        get("HIVY_MICROSANDBOX_ADDR", ":8080"),
		DatabaseDSN:                 os.Getenv("HIVY_MICROSANDBOX_DATABASE_DSN"),
		SentryDSN:                   os.Getenv("HIVY_MICROSANDBOX_SENTRY_DSN"),
		DatabaseMaxOpenConns:        integer("HIVY_MICROSANDBOX_DATABASE_MAX_OPEN_CONNS", 32),
		DatabaseMaxIdleConns:        integer("HIVY_MICROSANDBOX_DATABASE_MAX_IDLE_CONNS", 16),
		DatabaseConnMaxLifetime:     duration("HIVY_MICROSANDBOX_DATABASE_CONN_MAX_LIFETIME", 30*time.Minute),
		SchedulerPlacementTimeout:   duration("HIVY_MICROSANDBOX_SCHEDULER_PLACEMENT_TIMEOUT", 5*time.Second),
		APIToken:                    os.Getenv("HIVY_MICROSANDBOX_API_TOKEN"),
		RunnerJoinSecret:            os.Getenv("HIVY_MICROSANDBOX_RUNNER_JOIN_SECRET"),
		RunnerAPIToken:              os.Getenv("HIVY_MICROSANDBOX_RUNNER_API_TOKEN"),
		PreviewBaseDomain:           get("HIVY_MICROSANDBOX_PREVIEW_BASE_DOMAIN", "preview.usehivy.local"),
		PreviewJWTSecret:            os.Getenv("HIVY_MICROSANDBOX_PREVIEW_JWT_SECRET"),
		PreviewPasswordKey:          os.Getenv("HIVY_MICROSANDBOX_PREVIEW_PASSWORD_KEY"),
		PreviewCacheURL:             strings.TrimRight(os.Getenv("HIVY_MICROSANDBOX_PREVIEW_CACHE_URL"), "/"),
		PreviewCacheToken:           os.Getenv("HIVY_MICROSANDBOX_PREVIEW_CACHE_TOKEN"),
		PreviewCacheSync:            duration("HIVY_MICROSANDBOX_PREVIEW_CACHE_SYNC_INTERVAL", time.Minute),
		HeartbeatInterval:           duration("HIVY_MICROSANDBOX_HEARTBEAT_INTERVAL", time.Minute),
		RunnerUnhealthyAfter:        duration("HIVY_MICROSANDBOX_RUNNER_UNHEALTHY_AFTER", 3*time.Minute),
		RunnerCheckInterval:         duration("HIVY_MICROSANDBOX_RUNNER_CHECK_INTERVAL", 5*time.Minute),
		ControlURL:                  strings.TrimRight(os.Getenv("HIVY_MICROSANDBOX_CONTROL_URL"), "/"),
		RunnerName:                  get("HIVY_MICROSANDBOX_RUNNER_NAME", "runner-local"),
		RunnerPublicURL:             strings.TrimRight(os.Getenv("HIVY_MICROSANDBOX_RUNNER_PUBLIC_URL"), "/"),
		RunnerPreviewBaseURL:        strings.TrimRight(os.Getenv("HIVY_MICROSANDBOX_RUNNER_PREVIEW_BASE_URL"), "/"),
		RunnerBackend:               get("HIVY_MICROSANDBOX_RUNNER_BACKEND", "microsandbox"),
		RunnerTotalCPU:              integer("HIVY_MICROSANDBOX_RUNNER_TOTAL_CPU", 4),
		RunnerTotalMemoryMB:         integer("HIVY_MICROSANDBOX_RUNNER_TOTAL_MEMORY_MB", 8192),
		RunnerTotalDiskGB:           integer("HIVY_MICROSANDBOX_RUNNER_TOTAL_DISK_GB", 200),
		RunnerCPUOvercommit:         float("HIVY_MICROSANDBOX_RUNNER_CPU_OVERCOMMIT", 1.5),
		RunnerMemoryOvercommit:      float("HIVY_MICROSANDBOX_RUNNER_MEMORY_OVERCOMMIT", 1),
		RunnerDiskOvercommit:        float("HIVY_MICROSANDBOX_RUNNER_DISK_OVERCOMMIT", 4),
		RunnerPreviewPortRangeStart: integer("HIVY_MICROSANDBOX_RUNNER_PREVIEW_PORT_RANGE_START", api.DefaultPreviewHostPortRangeStart),
		RunnerPreviewPortRangeEnd:   integer("HIVY_MICROSANDBOX_RUNNER_PREVIEW_PORT_RANGE_END", api.DefaultPreviewHostPortRangeEnd),
		RunnerDNSNameserver:         strings.TrimSpace(os.Getenv("HIVY_MICROSANDBOX_RUNNER_DNS_NAMESERVER")),
		RunnerPrivateEgressCIDR:     strings.TrimSpace(os.Getenv("HIVY_MICROSANDBOX_RUNNER_PRIVATE_EGRESS_CIDR")),
		RunnerPrivateEgressPorts:    strings.TrimSpace(os.Getenv("HIVY_MICROSANDBOX_RUNNER_PRIVATE_EGRESS_PORTS")),
		ImageRegistry:               get("HIVY_MICROSANDBOX_IMAGE_REGISTRY", "10.80.0.3:5000"),
	}
}

func get(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func integer(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func float(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func duration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
