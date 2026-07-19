#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
config_root="${repo_root}/kubernetes/config"
s3_env="${config_root}/env/infrastructure/hetzner-s3.env"
staging_dir="${config_root}/env/staging"
production_dir="${config_root}/env/production"
mode="${1:-initial}"

if [[ "${mode}" != "initial" && "${mode}" != "--refresh-backups" ]]; then
  echo "usage: $0 [--refresh-backups]" >&2
  exit 1
fi

if [[ ! -f "${s3_env}" ]]; then
  echo "missing ${s3_env}" >&2
  exit 1
fi

read_env() {
  local key="$1"
  awk -v key="${key}" 'index($0, key "=") == 1 { print substr($0, length(key) + 2); found = 1 } END { if (!found) exit 1 }' "${s3_env}"
}

write_secret() {
  local destination="$1"
  shift
  if [[ -e "${destination}" && "${mode}" != "--refresh-backups" ]]; then
    echo "refusing to overwrite existing secret file: ${destination}" >&2
    exit 1
  fi
  printf '%s\n' "$@" > "${destination}"
  chmod 600 "${destination}"
}

mkdir -p "${staging_dir}" "${production_dir}"
umask 077

staging_pg_access="$(read_env HETZNER_S3_STAGING_PG_BACKEND_ACCESS_KEY_ID)"
staging_pg_secret="$(read_env HETZNER_S3_STAGING_PG_BACKEND_SECRET_ACCESS_KEY)"
staging_qdrant_access="$(read_env HETZNER_S3_STAGING_QDRANT_ACCESS_KEY_ID)"
staging_qdrant_secret="$(read_env HETZNER_S3_STAGING_QDRANT_SECRET_ACCESS_KEY)"

prod_pg_access="$(read_env HETZNER_S3_PROD_PG_BACKEND_ACCESS_KEY_ID)"
prod_pg_secret="$(read_env HETZNER_S3_PROD_PG_BACKEND_SECRET_ACCESS_KEY)"
prod_qdrant_access="$(read_env HETZNER_S3_PROD_QDRANT_ACCESS_KEY_ID)"
prod_qdrant_secret="$(read_env HETZNER_S3_PROD_QDRANT_SECRET_ACCESS_KEY)"

write_secret "${staging_dir}/postgres-backup.env" \
  "ACCESS_KEY_ID=${staging_pg_access}" \
  "ACCESS_SECRET_KEY=${staging_pg_secret}"
write_secret "${staging_dir}/qdrant-backup.env" \
  "ACCESS_KEY_ID=${staging_qdrant_access}" \
  "ACCESS_SECRET_KEY=${staging_qdrant_secret}"
write_secret "${staging_dir}/redis-backup.env" \
  "AWS_ACCESS_KEY_ID=${staging_pg_access}" \
  "AWS_SECRET_ACCESS_KEY=${staging_pg_secret}" \
  "AWS_DEFAULT_REGION=hel1" \
  "S3_ENDPOINT=https://hel1.your-objectstorage.com" \
  "S3_BUCKET=usehivy-staging-pg-backend" \
  "S3_PREFIX=redis"

if [[ "${mode}" == "initial" ]]; then
  write_secret "${production_dir}/postgres.env" \
    "username=hivy" \
    "password=$(openssl rand -hex 32)"
  write_secret "${production_dir}/redis.env" \
    "password=$(openssl rand -hex 32)"
  write_secret "${production_dir}/qdrant.env" \
    "HIVY_QDRANT_API_KEY=$(openssl rand -hex 32)"
fi
write_secret "${production_dir}/postgres-backup.env" \
  "ACCESS_KEY_ID=${prod_pg_access}" \
  "ACCESS_SECRET_KEY=${prod_pg_secret}"
write_secret "${production_dir}/qdrant-backup.env" \
  "ACCESS_KEY_ID=${prod_qdrant_access}" \
  "ACCESS_SECRET_KEY=${prod_qdrant_secret}"
write_secret "${production_dir}/redis-backup.env" \
  "AWS_ACCESS_KEY_ID=${prod_pg_access}" \
  "AWS_SECRET_ACCESS_KEY=${prod_pg_secret}" \
  "AWS_DEFAULT_REGION=hel1" \
  "S3_ENDPOINT=https://hel1.your-objectstorage.com" \
  "S3_BUCKET=usehivy-prod-pg-backend" \
  "S3_PREFIX=redis"

echo "generated staging and production backup secrets"
if [[ "${mode}" == "initial" ]]; then
  echo "generated production data-plane credentials"
fi
