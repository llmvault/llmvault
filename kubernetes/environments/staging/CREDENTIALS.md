# Staging credential checklist

Staging exercises production code paths with independent or test credentials.
Do not copy production secrets. Put every backend value below in the ignored
`secrets/backend.env` file, then run `secrets/validate.sh` before rendering or
applying the overlay.

## Required before deployment

### Paystack test mode

- `HIVY_PAYSTACK_SECRET_KEY`: the `sk_test_...` key from Paystack test mode.
- Configure the test-mode webhook URL as
  `https://staging.api.usehivy.com/internal/webhooks/paystack`.

There is no frontend Paystack public-key variable. The backend creates the
transaction and the web application resumes it using the returned access code.
Paystack signs webhooks with the same secret key.

### Social login applications

Create separate staging OAuth applications and put each ID and secret in
`secrets/backend.env`:

- GitHub: `HIVY_OAUTH_GITHUB_CLIENT_ID` and
  `HIVY_OAUTH_GITHUB_CLIENT_SECRET`; callback
  `https://staging.api.usehivy.com/oauth/github/callback`.
- Google: `HIVY_OAUTH_GOOGLE_CLIENT_ID` and
  `HIVY_OAUTH_GOOGLE_CLIENT_SECRET`; callback
  `https://staging.api.usehivy.com/oauth/google/callback`.
- X: `HIVY_OAUTH_X_CLIENT_ID` and `HIVY_OAUTH_X_CLIENT_SECRET`; callback
  `https://staging.api.usehivy.com/oauth/x/callback`.

### Email

- Create a staging-specific Resend API key usable through SMTP and put it in
  `HIVY_SMTP_PASSWORD`.
- Verify that `staging@notifications.usehivy.com` is allowed by the Resend
  sending domain. The non-secret SMTP settings are in `backend-config.yaml`.

### Microsandbox

- Put the shared in-cluster control-plane API token in
  `HIVY_MICROSANDBOX_CONTROL_API_TOKEN`. This is an internal secret that will
  be generated with the Microsandbox control-plane manifests, not a vendor
  credential you need to purchase or create.
- Staging and production share the control plane, runner fleet, image versions,
  and current single control API token. Their Hivy application databases and
  sandbox ownership records remain separate.

### Web tools

Create staging keys, or dedicated restricted keys, for:

- `HIVY_SPIDER_CLOUD_API_KEY`
- `HIVY_FIRECRAWL_API_KEY`
- `HIVY_SERPER_API_KEY`

All three are required because production uses Spider then Firecrawl for
scrape/crawl/map and Serper then Firecrawl for search.

### Application object storage

Create a separate staging S3-compatible bucket and provide:

- `HIVY_AWS_DEFAULT_REGION`
- `HIVY_AWS_ENDPOINT_URL`
- `HIVY_AWS_PRESIGN_ENDPOINT_URL`
- `HIVY_AWS_S3_BUCKET_NAME`
- `HIVY_AWS_ACCESS_KEY_ID`
- `HIVY_AWS_SECRET_ACCESS_KEY`

The bucket must allow browser requests from `https://staging.usehivy.com` using
the signed upload and download methods used by the application.

### Observability and RAG

- `HIVY_SENTRY_DSN`: backend staging project or staging-enabled DSN.
- `HIVY_AGENT_SANDBOX_SENTRY_DSN`: agent sandbox staging project or DSN.
- `HIVY_LLM_API_KEY`: OpenRouter key for embeddings.
- `HIVY_RERANKER_API_KEY`: OpenRouter key for reranking; it may be the same
  restricted key if desired.
- `HIVY_GITHUB_TOKEN`: token used by the skill hydrator to avoid anonymous API
  rate limits.

Qdrant is internal and needs no external vendor credential. Its generated API
key lives in the ignored `secrets/qdrant.env` file and is rendered into both
the Qdrant Secret and the backend Secret. For a workspace whose other secret
files already exist, generate only this new key with:

```sh
kubernetes/environments/staging/secrets/generate-qdrant.sh
```

The staging Qdrant workload must be healthy at `qdrant:6334` before the
backend workloads are applied.

### Nango

The generated `HIVY_NANGO_SECRET_KEY` and `HIVY_NANGO_WEBHOOKS_SECRET` are
already staging-specific. The Nango deployment must use the matching API and
webhook values. Its own encryption key, dashboard credentials, database, and
provider configurations belong in the forthcoming Nango manifests, not this
backend Secret.

Create or prepare staging credentials for the production integration set:

- `github-app`: GitHub App ID, app link, private key, and webhook secret.
- `github-app-code-reviews`: a separate GitHub App ID, app link, private key,
  and webhook secret.
- `linear`: OAuth client ID, client secret, scopes, and webhook secret.
- `notion`: OAuth client ID, client secret, and webhook secret.
- `railway`: OAuth client ID, client secret, and scopes.
- `slack`: OAuth client ID, client secret, and scopes.

The staging Nango public URL will be
`https://staging.connections.usehivy.com`; use its generated callback and
webhook URLs when configuring those provider applications. Do not reuse the
production OAuth applications because their callback/webhook configuration and
connection data belong to production.

Also create the `apify`, `bugsink`, `glitchtip`, and `vercel` Nango provider
configurations. They do not have platform credential fields in the current
production configuration; users supply their API credentials when connecting.

Model-provider keys are intentionally excluded. They are used only by local
and E2E testing and are not required in staging or production.
