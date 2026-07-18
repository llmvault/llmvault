#!/usr/bin/env bash
set -euo pipefail

secret_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
backend_env="${1:-${secret_dir}/backend.env}"
qdrant_env="${2:-${secret_dir}/qdrant.env}"

if [[ ! -f "${backend_env}" ]]; then
  echo "missing staging backend secret input: ${backend_env}" >&2
  exit 1
fi

if [[ ! -f "${qdrant_env}" ]]; then
  echo "missing staging Qdrant secret input: ${qdrant_env}" >&2
  exit 1
fi

required=(
  HIVY_KMS_KEY
  HIVY_JWT_SIGNING_KEY
  HIVY_AUTH_RSA_PRIVATE_KEY
  HIVY_NANGO_SECRET_KEY
  HIVY_NANGO_WEBHOOKS_SECRET
  HIVY_SANDBOX_ENCRYPTION_KEY
  HIVY_PREVIEW_ACTIVITY_TOKEN
  HIVY_ADMIN_SECRET
  HIVY_MICROSANDBOX_CONTROL_API_TOKEN
  HIVY_PAYSTACK_SECRET_KEY
  HIVY_SMTP_PASSWORD
  HIVY_OAUTH_GITHUB_CLIENT_ID
  HIVY_OAUTH_GITHUB_CLIENT_SECRET
  HIVY_OAUTH_GOOGLE_CLIENT_ID
  HIVY_OAUTH_GOOGLE_CLIENT_SECRET
  HIVY_OAUTH_X_CLIENT_ID
  HIVY_OAUTH_X_CLIENT_SECRET
  HIVY_SPIDER_CLOUD_API_KEY
  HIVY_FIRECRAWL_API_KEY
  HIVY_SERPER_API_KEY
  HIVY_AWS_DEFAULT_REGION
  HIVY_AWS_ENDPOINT_URL
  HIVY_AWS_PRESIGN_ENDPOINT_URL
  HIVY_AWS_S3_BUCKET_NAME
  HIVY_AWS_ACCESS_KEY_ID
  HIVY_AWS_SECRET_ACCESS_KEY
  HIVY_SENTRY_DSN
  HIVY_AGENT_SANDBOX_SENTRY_DSN
  HIVY_LLM_API_KEY
  HIVY_RERANKER_API_KEY
  HIVY_GITHUB_TOKEN
)

missing=()
for key in "${required[@]}"; do
  value="$(awk -v key="${key}" 'index($0, key "=") == 1 { value = substr($0, length(key) + 2) } END { print value }' "${backend_env}")"
  if [[ -z "${value}" ]]; then
    missing+=("${key}")
  fi
done

if (( ${#missing[@]} > 0 )); then
  echo "staging is missing required credentials:" >&2
  printf '  %s\n' "${missing[@]}" >&2
  exit 1
fi

paystack_key="$(awk 'index($0, "HIVY_PAYSTACK_SECRET_KEY=") == 1 { value = substr($0, length("HIVY_PAYSTACK_SECRET_KEY=") + 1) } END { print value }' "${backend_env}")"
if [[ "${paystack_key}" != sk_test_* ]]; then
  echo "HIVY_PAYSTACK_SECRET_KEY must be a Paystack test key (sk_test_...)" >&2
  exit 1
fi

qdrant_api_key="$(awk 'index($0, "HIVY_QDRANT_API_KEY=") == 1 { value = substr($0, length("HIVY_QDRANT_API_KEY=") + 1) } END { print value }' "${qdrant_env}")"
if (( ${#qdrant_api_key} < 32 )); then
  echo "HIVY_QDRANT_API_KEY must be at least 32 characters" >&2
  exit 1
fi

echo "staging backend credential inputs are complete"
