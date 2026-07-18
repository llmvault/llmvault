#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
secret_dir="$repo_root/kubernetes/environments/production/secrets"
s3_env="$repo_root/.env.hetzner-s3"
railway_project="55776e03-e6c2-4a9b-828b-4e759495aa70"
railway_environment="3c177170-0fb2-4dcb-a034-12676bb242c6"
railway_service="a9b4e563-a9a6-4e69-92d4-fc8819435f80"

for command in jq openssl railway; do
  command -v "$command" >/dev/null || { echo "$command is required" >&2; exit 1; }
done
[[ -f "$s3_env" ]] || { echo "$s3_env is required" >&2; exit 1; }

control_file="$secret_dir/microsandbox-control.env"
postgres_file="$secret_dir/microsandbox-postgres.env"
backup_file="$secret_dir/microsandbox-postgres-backup.env"
for file in "$control_file" "$postgres_file" "$backup_file"; do
  if [[ -e "$file" ]]; then
    echo "refusing to overwrite existing secret file: $file" >&2
    exit 1
  fi
done

railway_json="$(mktemp)"
cleanup() {
  rm -f "$railway_json"
}
trap cleanup EXIT

railway variable list \
  --project "$railway_project" \
  --environment "$railway_environment" \
  --service "$railway_service" \
  --json >"$railway_json"

required_railway_keys=(
  HIVY_MICROSANDBOX_API_TOKEN
  HIVY_MICROSANDBOX_RUNNER_JOIN_SECRET
  HIVY_MICROSANDBOX_RUNNER_API_TOKEN
  HIVY_MICROSANDBOX_PREVIEW_JWT_SECRET
  HIVY_MICROSANDBOX_PREVIEW_PASSWORD_KEY
  HIVY_MICROSANDBOX_PREVIEW_CACHE_TOKEN
)
for key in "${required_railway_keys[@]}"; do
  [[ "$(jq -r --arg key "$key" '.[$key] // empty' "$railway_json")" != "" ]] || {
    echo "Railway variable $key is missing" >&2
    exit 1
  }
done

set -a
# shellcheck disable=SC1090
source "$s3_env"
set +a
: "${HETZNER_S3_PROD_PG_MICROSANDBOX_ACCESS_KEY_ID:?missing Microsandbox backup access key}"
: "${HETZNER_S3_PROD_PG_MICROSANDBOX_SECRET_ACCESS_KEY:?missing Microsandbox backup secret key}"

db_password="$(openssl rand -base64 36 | tr -d '=+/\n' | head -c 40)"
umask 077
install -m 0600 /dev/null "$postgres_file"
install -m 0600 /dev/null "$control_file"
install -m 0600 /dev/null "$backup_file"

printf 'username=microsandbox\npassword=%s\n' "$db_password" >>"$postgres_file"
printf '%s\n' \
  "HIVY_MICROSANDBOX_DATABASE_DSN=postgresql://microsandbox:${db_password}@microsandbox-postgres-rw.production.svc.cluster.local:5432/microsandbox?sslmode=require" \
  >>"$control_file"
for key in "${required_railway_keys[@]}"; do
  value="$(jq -r --arg key "$key" '.[$key]' "$railway_json")"
  printf '%s=%s\n' "$key" "$value" >>"$control_file"
done
sentry_dsn="$(jq -r '.HIVY_MICROSANDBOX_SENTRY_DSN // empty' "$railway_json")"
printf 'HIVY_MICROSANDBOX_SENTRY_DSN=%s\n' "$sentry_dsn" >>"$control_file"

printf 'ACCESS_KEY_ID=%s\nACCESS_SECRET_KEY=%s\n' \
  "$HETZNER_S3_PROD_PG_MICROSANDBOX_ACCESS_KEY_ID" \
  "$HETZNER_S3_PROD_PG_MICROSANDBOX_SECRET_ACCESS_KEY" \
  >>"$backup_file"

echo "Generated ignored Microsandbox Kubernetes secrets in $secret_dir"
