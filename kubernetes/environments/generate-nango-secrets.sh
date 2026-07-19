#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
s3_env="${repo_root}/.env.hetzner-s3"
staging_dir="${repo_root}/kubernetes/environments/staging/secrets"
production_dir="${repo_root}/kubernetes/environments/production/secrets"
expected_project_id="55776e03-e6c2-4a9b-828b-4e759495aa70"

for command_name in railway jq openssl; do
  if ! command -v "${command_name}" >/dev/null; then
    echo "missing required command: ${command_name}" >&2
    exit 1
  fi
done

if [[ ! -f "${s3_env}" ]]; then
  echo "missing ${s3_env}" >&2
  exit 1
fi

project_id="$(railway status --json | jq -er '.id')"
if [[ "${project_id}" != "${expected_project_id}" ]]; then
  echo "Railway is linked to unexpected project ${project_id}" >&2
  exit 1
fi

read_s3_env() {
  local key="$1"
  awk -v key="${key}" 'index($0, key "=") == 1 { print substr($0, length(key) + 2); found = 1 } END { if (!found) exit 1 }' "${s3_env}"
}

read_json_value() {
  local document="$1"
  local key="$2"
  jq -er --arg key "${key}" '.[$key] | select(type == "string" and length > 0)' <<<"${document}"
}

write_secret() {
  local destination="$1"
  shift
  if [[ -e "${destination}" ]]; then
    echo "refusing to overwrite existing secret file: ${destination}" >&2
    exit 1
  fi
  printf '%s\n' "$@" >"${destination}"
  chmod 600 "${destination}"
}

mkdir -p "${staging_dir}" "${production_dir}"
umask 077

nango_vars="$(railway variable list --service connections.usehivy.com --environment production --json)"
api_vars="$(railway variable list --service api.usehivy.com --environment production --json)"

nango_encryption_key="$(read_json_value "${nango_vars}" NANGO_ENCRYPTION_KEY)"
nango_dashboard_username="$(read_json_value "${nango_vars}" NANGO_DASHBOARD_USERNAME)"
nango_dashboard_password="$(read_json_value "${nango_vars}" NANGO_DASHBOARD_PASSWORD)"
nango_secret_key="$(read_json_value "${api_vars}" HIVY_NANGO_SECRET_KEY)"
nango_webhooks_secret="$(read_json_value "${api_vars}" HIVY_NANGO_WEBHOOKS_SECRET)"

for environment_name in staging production; do
  if [[ "${environment_name}" == "staging" ]]; then
    secret_dir="${staging_dir}"
    s3_prefix="HETZNER_S3_STAGING_PG_NANGO"
  else
    secret_dir="${production_dir}"
    s3_prefix="HETZNER_S3_PROD_PG_NANGO"
  fi

  write_secret "${secret_dir}/nango-runtime.env" \
    "NANGO_ENCRYPTION_KEY=${nango_encryption_key}" \
    "NANGO_DASHBOARD_USERNAME=${nango_dashboard_username}" \
    "NANGO_DASHBOARD_PASSWORD=${nango_dashboard_password}"
  write_secret "${secret_dir}/nango-backend.env" \
    "HIVY_NANGO_SECRET_KEY=${nango_secret_key}" \
    "HIVY_NANGO_WEBHOOKS_SECRET=${nango_webhooks_secret}"
  write_secret "${secret_dir}/nango-postgres.env" \
    "username=nango" \
    "password=$(openssl rand -hex 32)"
  write_secret "${secret_dir}/nango-postgres-backup.env" \
    "ACCESS_KEY_ID=$(read_s3_env "${s3_prefix}_ACCESS_KEY_ID")" \
    "ACCESS_SECRET_KEY=$(read_s3_env "${s3_prefix}_SECRET_ACCESS_KEY")"
done

echo "generated staging and production Nango secrets"
