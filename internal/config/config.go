package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Environment string `env:"HIVY_ENVIRONMENT" envDefault:"development"` // "development" or "production"

	Port      int    `env:"HIVY_PORT,required"`
	LogLevel  string `env:"HIVY_LOG_LEVEL,required"`
	LogFormat string `env:"HIVY_LOG_FORMAT,required"`

	DBHost     string `env:"HIVY_DB_HOST,required"`
	DBPort     int    `env:"HIVY_DB_PORT" envDefault:"5432"`
	DBUser     string `env:"HIVY_DB_USER,required"`
	DBPassword string `env:"HIVY_DB_PASSWORD,required"`
	DBName     string `env:"HIVY_DB_NAME,required"`
	DBSSLMode  string `env:"HIVY_DB_SSLMODE" envDefault:"disable"`

	KMSType   string `env:"HIVY_KMS_TYPE,required"` // "aead" or "awskms"
	KMSKey    string `env:"HIVY_KMS_KEY"`           // base64-encoded 32-byte key (aead) or AWS KMS key ID/ARN (awskms)
	AWSRegion string `env:"HIVY_AWS_REGION"`        // AWS region for awskms (default: us-east-1)

	RedisURL      string        `env:"HIVY_REDIS_URL"`  // Full URL (e.g. rediss://...), enables TLS automatically
	RedisAddr     string        `env:"HIVY_REDIS_ADDR"` // Fallback: host:port (ignored when HIVY_REDIS_URL is set)
	RedisPassword string        `env:"HIVY_REDIS_PASSWORD"`
	RedisDB       int           `env:"HIVY_REDIS_DB"`
	RedisCacheTTL time.Duration `env:"HIVY_REDIS_CACHE_TTL,required"`

	MemCacheTTL     time.Duration `env:"HIVY_MEM_CACHE_TTL,required"`
	MemCacheMaxSize int           `env:"HIVY_MEM_CACHE_MAX_SIZE,required"`

	JWTSigningKey string `env:"HIVY_JWT_SIGNING_KEY,required"`

	AuthRSAPrivateKey   string        `env:"HIVY_AUTH_RSA_PRIVATE_KEY,required"` // base64-encoded PEM
	AuthIssuer          string        `env:"HIVY_AUTH_ISSUER" envDefault:"hivy"`
	AuthAudience        string        `env:"HIVY_AUTH_AUDIENCE" envDefault:"https://api.usehivy.com"`
	AuthAccessTokenTTL  time.Duration `env:"HIVY_AUTH_ACCESS_TOKEN_TTL" envDefault:"15m"`
	AuthRefreshTokenTTL time.Duration `env:"HIVY_AUTH_REFRESH_TOKEN_TTL" envDefault:"720h"` // 30 days

	FrontendURL string `env:"HIVY_FRONTEND_URL,required"`

	// Auth: auto-confirm email on registration (useful for self-hosted deployments)
	AutoConfirmEmail bool `env:"HIVY_AUTO_CONFIRM_EMAIL" envDefault:"false"`

	// Email (SMTP transactional delivery). Provider-agnostic — point at any SMTP
	// server (Resend SMTP, SES, Postmark, self-hosted…). When SMTPHost is empty
	// the worker renders each email and writes it to a temp file (logging the
	// path) instead of sending, so local dev needs no mail provider.
	SMTPHost     string `env:"HIVY_SMTP_HOST"`
	SMTPPort     int    `env:"HIVY_SMTP_PORT" envDefault:"587"`
	SMTPUsername string `env:"HIVY_SMTP_USERNAME"`
	SMTPPassword string `env:"HIVY_SMTP_PASSWORD"`
	SMTPTLS      string `env:"HIVY_SMTP_TLS" envDefault:"starttls"` // starttls | ssl | none
	EmailFrom    string `env:"HIVY_EMAIL_FROM"`                     // e.g. "Acme <hello@acme.com>"; required for SMTP delivery
	// Substituted into templates as {{{siteUrl}}} (footer links) and
	// {{{assetBaseUrl}}} (logo image base).
	EmailSiteURL  string `env:"HIVY_EMAIL_SITE_URL"`
	EmailAssetURL string `env:"HIVY_EMAIL_ASSET_URL"`

	OAuthGitHubClientID     string `env:"HIVY_OAUTH_GITHUB_CLIENT_ID"`
	OAuthGitHubClientSecret string `env:"HIVY_OAUTH_GITHUB_CLIENT_SECRET"`
	OAuthGoogleClientID     string `env:"HIVY_OAUTH_GOOGLE_CLIENT_ID"`
	OAuthGoogleClientSecret string `env:"HIVY_OAUTH_GOOGLE_CLIENT_SECRET"`
	OAuthXClientID          string `env:"HIVY_OAUTH_X_CLIENT_ID"`
	OAuthXClientSecret      string `env:"HIVY_OAUTH_X_CLIENT_SECRET"`

	CORSOrigins []string `env:"HIVY_CORS_ORIGINS" envSeparator:","`

	// Trusted reverse-proxy hops (CIDRs). Forwarding headers are honoured only when the peer falls
	// inside one of these; loopback (nginx) is trusted by default.
	TrustedProxyCIDRs []string `env:"HIVY_TRUSTED_PROXY_CIDRS" envSeparator:"," envDefault:"127.0.0.0/8,::1/128"`

	NangoEndpoint       string `env:"HIVY_NANGO_ENDPOINT"`                 // e.g. http://localhost:3004
	NangoSecretKey      string `env:"HIVY_NANGO_SECRET_KEY"`               // Nango secret key for API auth
	NangoWebhooksSecret string `env:"HIVY_NANGO_WEBHOOKS_SECRET,required"` // Nango secret key for webhook signature verification

	// GitHub API token used by the skill hydrator. Optional — raises the
	// anonymous rate limit from 60 req/hr to 5000 req/hr per token.
	GitHubToken string `env:"HIVY_GITHUB_TOKEN"`

	MCPPort    int    `env:"HIVY_MCP_PORT" envDefault:"8081"`
	MCPBaseURL string `env:"HIVY_MCP_BASE_URL" envDefault:"http://localhost:8081"`
	// MCPOAuthCallbackURL is the public callback for arbitrary remote MCP OAuth
	// servers. Empty falls back to HIVY_API_WEBHOOK_BASE_URL plus the standard
	// callback path so self-hosted deployments never depend on a Hivy domain.
	MCPOAuthCallbackURL string `env:"HIVY_MCP_OAUTH_CALLBACK_URL"`

	// Sandbox provider (global — one provider for the whole platform)
	SandboxEncryptionKey              string `env:"HIVY_SANDBOX_ENCRYPTION_KEY"` // base64-encoded 32-byte key for encrypting sandbox secrets
	SandboxProviderID                 string `env:"HIVY_SANDBOX_PROVIDER_ID"`    // empty disables sandbox orchestration
	SandboxDockerHost                 string `env:"HIVY_SANDBOX_DOCKER_HOST"`
	SandboxDockerRuntimeOrigin        string `env:"HIVY_SANDBOX_DOCKER_RUNTIME_ORIGIN"`
	SandboxDockerControlOrigin        string `env:"HIVY_SANDBOX_DOCKER_CONTROL_ORIGIN"`
	SandboxDockerContainerLabelPrefix string `env:"HIVY_SANDBOX_DOCKER_CONTAINER_LABEL_PREFIX" envDefault:"hivy"`
	// SandboxDockerSystemd boots docker sandboxes with systemd as PID 1
	// (/sbin/init entrypoint, private cgroupns, SIGRTMIN+3 stop signal), the
	// same process model as production microsandbox VMs. Off = legacy tini
	// entrypoint mode.
	SandboxDockerSystemd        bool   `env:"HIVY_SANDBOX_DOCKER_SYSTEMD"`
	MicrosandboxControlURL      string `env:"HIVY_MICROSANDBOX_CONTROL_URL"`
	MicrosandboxControlAPIToken string `env:"HIVY_MICROSANDBOX_CONTROL_API_TOKEN"`

	RailwayAPIToken              string `env:"HIVY_RAILWAY_API_TOKEN"`
	RailwayProjectID             string `env:"HIVY_RAILWAY_PROJECT_ID"`
	RailwayEnvironmentID         string `env:"HIVY_RAILWAY_ENVIRONMENT_ID"`
	RailwayRegion                string `env:"HIVY_RAILWAY_REGION"`
	RailwayRuntimePort           int    `env:"HIVY_RAILWAY_RUNTIME_PORT" envDefault:"7080"`
	SandboxWarmPoolDefaultSize   int    `env:"HIVY_SANDBOX_WARM_POOL_DEFAULT_SIZE" envDefault:"0"`
	SandboxWarmPoolDeveloperSize int    `env:"HIVY_SANDBOX_WARM_POOL_DEVELOPER_SIZE" envDefault:"0"`

	DaytonaAPIURL string `env:"HIVY_DAYTONA_API_URL"`
	DaytonaAPIKey string `env:"HIVY_DAYTONA_API_KEY"`
	DaytonaTarget string `env:"HIVY_DAYTONA_TARGET"`

	APIWebhookBaseURL string `env:"HIVY_API_WEBHOOK_BASE_URL" envDefault:"https://api.usehivy.com"` // public API base URL for provider webhook callbacks
	ProxyHost         string `env:"HIVY_PROXY_HOST" envDefault:"proxy.usehivy.com"`                 // LLM proxy hostname (proxy.usehivy.com)

	RuntimeRedisStreamShardCount int `env:"HIVY_RUNTIME_REDIS_STREAM_SHARD_COUNT" envDefault:"64"`

	SandboxesRuntimeImageTag string `env:"HIVY_SANDBOXES_RUNTIME_IMAGE_TAG"`
	// SandboxesAppImageTag pins the app sandbox image (hivy-appd host, built
	// from sandboxes/app/Dockerfile). Empty falls back to :latest, mirroring
	// HIVY_SANDBOXES_RUNTIME_IMAGE_TAG.
	SandboxesAppImageTag string `env:"HIVY_SANDBOXES_APP_IMAGE_TAG"`

	// Browser setup/admin panel. When disabled, admin routes are not mounted.
	AdminEnabled bool   `env:"HIVY_ADMIN_ENABLED" envDefault:"false"`
	AdminSecret  string `env:"HIVY_ADMIN_SECRET"`

	// PreviewBaseDomain is the wildcard preview host suffix
	// ({port}-{sandbox_id}.<domain>) for user-facing sandbox previews, injected
	// into the agent system prompt and the apps preview URL builder. Required:
	// the server fails fast at startup if unset, so downstream code never has to
	// guard against an empty preview domain. Self-hosters set their own.
	PreviewBaseDomain string `env:"HIVY_PREVIEW_BASE_DOMAIN,required"`

	PreviewCNAMETarget string `env:"HIVY_PREVIEW_CNAME_TARGET" envDefault:"preview-proxy.usehivy.com"`
	AcmeDNSAPIURL      string `env:"HIVY_ACME_DNS_API_URL"` // acme-dns registration API (e.g. https://acme-dns-api.daytona.example.com)
	CaddyAdminURL      string `env:"HIVY_CADDY_ADMIN_URL"`  // Caddy admin API proxy (e.g. https://caddy-admin.daytona.example.com)

	// Web crawl/search providers, enabled by their API keys. Priority per
	// operation is hardcoded in bootstrap: scrape/crawl/map go spider then
	// firecrawl; search goes serper then firecrawl (spider is not used for
	// search).
	SpiderAPIKey    string `env:"HIVY_SPIDER_CLOUD_API_KEY"` // empty = spider disabled
	FirecrawlAPIKey string `env:"HIVY_FIRECRAWL_API_KEY"`    // empty = firecrawl disabled
	SerperAPIKey    string `env:"HIVY_SERPER_API_KEY"`       // empty = serper disabled

	// S3 (agent drive storage and uploads; empty HIVY_AWS_S3_BUCKET_NAME disables storage-backed uploads)
	S3Bucket          string `env:"HIVY_AWS_S3_BUCKET_NAME"`
	S3Region          string `env:"HIVY_AWS_DEFAULT_REGION" envDefault:"us-east-1"`
	S3Endpoint        string `env:"HIVY_AWS_ENDPOINT_URL"` // for MinIO / R2 / local dev
	S3PresignEndpoint string `env:"HIVY_AWS_PRESIGN_ENDPOINT_URL"`
	S3AccessKey       string `env:"HIVY_AWS_ACCESS_KEY_ID"`
	S3SecretKey       string `env:"HIVY_AWS_SECRET_ACCESS_KEY"`

	SandboxResourceCheckInterval time.Duration `env:"HIVY_SANDBOX_RESOURCE_CHECK_INTERVAL" envDefault:"30m"`
	SandboxIdleTimeout           time.Duration `env:"HIVY_SANDBOX_IDLE_TIMEOUT" envDefault:"5m"`
	AgentScheduleScanInterval    time.Duration `env:"HIVY_AGENT_SCHEDULE_SCAN_INTERVAL" envDefault:"5s"`
	PreviewActivityToken         string        `env:"HIVY_PREVIEW_ACTIVITY_TOKEN"`

	WorkerHealthPort     int           `env:"HIVY_WORKER_HEALTH_PORT" envDefault:"8090"`
	AsynqConcurrency     int           `env:"HIVY_ASYNQ_CONCURRENCY" envDefault:"30"`
	AsynqShutdownTimeout time.Duration `env:"HIVY_ASYNQ_SHUTDOWN_TIMEOUT" envDefault:"120s"`

	// Asynqmon dashboard. It exposes every queued/archived task payload (customer
	// messages, webhooks, emails), so it is disabled by default, requires
	// basic-auth, and is never mounted on the public health port.
	AsynqmonEnabled  bool   `env:"HIVY_ASYNQMON_ENABLED" envDefault:"false"`
	AsynqmonPort     int    `env:"HIVY_ASYNQMON_PORT" envDefault:"8091"`
	AsynqmonUser     string `env:"HIVY_ASYNQMON_USER"`
	AsynqmonPassword string `env:"HIVY_ASYNQMON_PASSWORD"`

	// Sentry error tracking + tracing (empty HIVY_SENTRY_DSN disables capture).
	SentryDSN                string  `env:"HIVY_SENTRY_DSN"`
	SentryEnabled            bool    `env:"HIVY_SENTRY_ENABLED" envDefault:"false"`
	SentryRelease            string  `env:"HIVY_SENTRY_RELEASE"`
	SentryTracesSampleRate   float64 `env:"HIVY_SENTRY_TRACES_SAMPLE_RATE" envDefault:"0.1"`
	SentryProfilesSampleRate float64 `env:"HIVY_SENTRY_PROFILES_SAMPLE_RATE" envDefault:"0.0"`
	AgentSandboxSentryDSN    string  `env:"HIVY_AGENT_SANDBOX_SENTRY_DSN"`

	// Qdrant (vector store, gRPC). Empty QdrantHost disables RAG.
	QdrantHost       string `env:"HIVY_QDRANT_HOST"`
	QdrantPort       int    `env:"HIVY_QDRANT_PORT" envDefault:"6334"`
	QdrantUseTLS     bool   `env:"HIVY_QDRANT_USE_TLS" envDefault:"false"`
	QdrantAPIKey     string `env:"HIVY_QDRANT_API_KEY"`
	QdrantCollection string `env:"HIVY_QDRANT_COLLECTION" envDefault:"rag_chunks_3072"`

	LLMAPIURL       string `env:"HIVY_LLM_API_URL"`
	LLMAPIKey       string `env:"HIVY_LLM_API_KEY"`
	LLMModel        string `env:"HIVY_LLM_MODEL"`
	LLMEmbeddingDim uint32 `env:"HIVY_LLM_EMBEDDING_DIM" envDefault:"3072"`

	MemoryEmbeddingModel string `env:"HIVY_MEMORY_EMBEDDING_MODEL" envDefault:"qwen/qwen3-embedding-8b"`
	MemoryEmbeddingDim   uint32 `env:"HIVY_MEMORY_EMBEDDING_DIM" envDefault:"1024"`

	RerankerBaseURL string `env:"HIVY_RERANKER_BASE_URL"`
	RerankerAPIKey  string `env:"HIVY_RERANKER_API_KEY"`
	RerankerModel   string `env:"HIVY_RERANKER_MODEL"`

	RagBatchSize int `env:"HIVY_RAG_BATCH_SIZE" envDefault:"100"`

	// Paystack (billing provider). Empty PaystackSecretKey disables it; checkout
	// then fails with ErrUnknownProvider. Plan prices live in the plan catalog.
	PaystackSecretKey string `env:"HIVY_PAYSTACK_SECRET_KEY"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if cfg.KMSType != "aead" && cfg.KMSType != "awskms" {
		return nil, fmt.Errorf("HIVY_KMS_TYPE must be 'aead' or 'awskms' (got %q)", cfg.KMSType)
	}

	if cfg.RedisURL == "" && cfg.RedisAddr == "" {
		return nil, fmt.Errorf("either HIVY_REDIS_URL or HIVY_REDIS_ADDR must be set")
	}
	if cfg.RuntimeRedisStreamShardCount <= 0 {
		cfg.RuntimeRedisStreamShardCount = 64
	}

	// Fail closed: an empty Nango webhook secret lets attackers forge signatures (HMAC with an
	// empty key). The `,required` tag misses an explicitly-empty value, so reject it here too.
	if strings.TrimSpace(cfg.NangoWebhooksSecret) == "" {
		return nil, fmt.Errorf("HIVY_NANGO_WEBHOOKS_SECRET must be set and non-empty")
	}

	// Fail fast on an empty preview domain (the `,required` tag misses an
	// explicitly-empty value). Guaranteeing it non-empty here lets the system
	// prompt and apps preview URL builder use it without their own guards.
	if strings.TrimSpace(cfg.PreviewBaseDomain) == "" {
		return nil, fmt.Errorf("HIVY_PREVIEW_BASE_DOMAIN must be set and non-empty")
	}

	cfg.CORSOrigins = includeFrontendCORSOrigin(cfg.CORSOrigins, cfg.FrontendURL)

	if cfg.IsProduction() && cfg.DBSSLMode == "disable" {
		// Config loads before logging is wired and can't import it (import cycle),
		// so the global default logger is the only option here.
		slog.Default().Warn("HIVY_DB_SSLMODE is 'disable' in production — database connections are unencrypted; set HIVY_DB_SSLMODE=require or HIVY_DB_SSLMODE=verify-full") //nolint:sloglint // startup warning before logging wired; import cycle prevents logging.FromContext
	}

	return cfg, nil
}

func includeFrontendCORSOrigin(origins []string, frontendURL string) []string {
	origin := URLOrigin(frontendURL)
	if origin == "" {
		return origins
	}
	for _, existing := range origins {
		if URLOrigin(existing) == origin {
			return origins
		}
	}
	return append(origins, origin)
}

func URLOrigin(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return raw
	}
	return parsed.Scheme + "://" + parsed.Host
}

// IsProduction reports whether the process is running in the production environment.
func (c *Config) IsProduction() bool {
	return c.Environment == "production"
}
