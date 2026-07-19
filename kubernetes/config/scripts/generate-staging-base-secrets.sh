#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
secret_dir="${repo_root}/kubernetes/config/env/staging"
backend_env="${secret_dir}/backend.env"
postgres_env="${secret_dir}/postgres.env"
redis_env="${secret_dir}/redis.env"
qdrant_env="${secret_dir}/qdrant.env"

for secret_file in "${backend_env}" "${postgres_env}" "${redis_env}" "${qdrant_env}"; do
  if [[ -e "${secret_file}" ]]; then
    echo "refusing to overwrite existing secret file: ${secret_file}" >&2
    exit 1
  fi
done

umask 077
rsa_pem="$(mktemp)"
trap 'rm -f "${rsa_pem}"' EXIT

openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 \
  -out "${rsa_pem}" 2>/dev/null

kms_key="$(openssl rand -base64 32 | tr -d '\n')"
jwt_key="$(openssl rand -base64 48 | tr -d '\n')"
rsa_key="$(openssl base64 -A -in "${rsa_pem}")"
nango_key="$(uuidgen | tr '[:upper:]' '[:lower:]')"
nango_webhook_key="$(openssl rand -base64 48 | tr -d '\n')"
sandbox_key="$(openssl rand -base64 32 | tr -d '\n')"
preview_token="$(openssl rand -base64 48 | tr -d '\n')"
admin_secret="$(openssl rand -base64 48 | tr -d '\n')"
postgres_password="$(openssl rand -hex 32)"
redis_password="$(openssl rand -hex 32)"
qdrant_api_key="$(openssl rand -hex 32)"

printf '%s\n' \
  "HIVY_KMS_KEY=${kms_key}" \
  "HIVY_JWT_SIGNING_KEY=${jwt_key}" \
  "HIVY_AUTH_RSA_PRIVATE_KEY=${rsa_key}" \
  "HIVY_NANGO_SECRET_KEY=${nango_key}" \
  "HIVY_NANGO_WEBHOOKS_SECRET=${nango_webhook_key}" \
  "HIVY_SANDBOX_ENCRYPTION_KEY=${sandbox_key}" \
  "HIVY_PREVIEW_ACTIVITY_TOKEN=${preview_token}" \
  "HIVY_ADMIN_SECRET=${admin_secret}" \
  > "${backend_env}"

printf '%s\n' \
  'username=hivy' \
  "password=${postgres_password}" \
  > "${postgres_env}"

printf '%s\n' \
  "password=${redis_password}" \
  > "${redis_env}"

printf '%s\n' \
  "HIVY_QDRANT_API_KEY=${qdrant_api_key}" \
  > "${qdrant_env}"

chmod 600 "${backend_env}" "${postgres_env}" "${redis_env}" "${qdrant_env}"
echo "generated staging Kubernetes secret inputs in ${secret_dir}"
