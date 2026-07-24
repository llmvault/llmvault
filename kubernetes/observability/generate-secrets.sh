#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
secret_dir="$repo_root/kubernetes/config/env/observability"
grafana_target="$secret_dir/grafana-admin.env"
telemetry_target="$secret_dir/telemetry-ingest.env"
datasource_target="$secret_dir/postgres-datasources.env"
qdrant_metrics_target="$secret_dir/qdrant-metrics.env"
production_postgres_target="$repo_root/kubernetes/config/env/production/postgres-observability.env"
staging_postgres_target="$repo_root/kubernetes/config/env/staging/postgres-observability.env"
production_qdrant_target="$repo_root/kubernetes/config/env/production/qdrant.env"
staging_qdrant_target="$repo_root/kubernetes/config/env/staging/qdrant.env"
refresh="${1:-}"

if [[ -n "$refresh" && "$refresh" != "--refresh" ]]; then
  echo "usage: $0 [--refresh]" >&2
  exit 2
fi

umask 077
mkdir -p "$secret_dir"

if [[ ! -e "$grafana_target" || "$refresh" == "--refresh" ]]; then
  password="$(openssl rand -base64 48 | tr -d '\n')"
  printf 'admin-user=admin\nadmin-password=%s\n' "$password" > "$grafana_target"
  chmod 600 "$grafana_target"
  echo "generated $grafana_target"
else
  echo "preserved $grafana_target"
fi

if [[ ! -e "$telemetry_target" || "$refresh" == "--refresh" ]]; then
  token="$(openssl rand -hex 48)"
  printf 'token=%s\n' "$token" > "$telemetry_target"
  chmod 600 "$telemetry_target"
  echo "generated $telemetry_target"
else
  echo "preserved $telemetry_target"
fi

read_password() {
  awk -F= '$1 == "password" { print substr($0, index($0, "=") + 1); exit }' "$1"
}

read_env_value() {
  local key="$1"
  local file="$2"
  awk -F= -v key="$key" '$1 == key { print substr($0, index($0, "=") + 1); exit }' "$file"
}

postgres_changed="false"
if [[ "$refresh" == "--refresh" || ! -e "$production_postgres_target" ]]; then
  production_postgres_password="$(openssl rand -base64 48 | tr -d '\n')"
  printf 'username=hivy_observability\npassword=%s\n' "$production_postgres_password" > "$production_postgres_target"
  chmod 600 "$production_postgres_target"
  postgres_changed="true"
  echo "generated $production_postgres_target"
else
  production_postgres_password="$(read_password "$production_postgres_target")"
  echo "preserved $production_postgres_target"
fi

if [[ "$refresh" == "--refresh" || ! -e "$staging_postgres_target" ]]; then
  staging_postgres_password="$(openssl rand -base64 48 | tr -d '\n')"
  printf 'username=hivy_observability\npassword=%s\n' "$staging_postgres_password" > "$staging_postgres_target"
  chmod 600 "$staging_postgres_target"
  postgres_changed="true"
  echo "generated $staging_postgres_target"
else
  staging_postgres_password="$(read_password "$staging_postgres_target")"
  echo "preserved $staging_postgres_target"
fi

if [[ "$refresh" == "--refresh" || "$postgres_changed" == "true" || ! -e "$datasource_target" ]]; then
  printf 'PRODUCTION_POSTGRES_PASSWORD=%s\nSTAGING_POSTGRES_PASSWORD=%s\n' \
    "$production_postgres_password" "$staging_postgres_password" > "$datasource_target"
  chmod 600 "$datasource_target"
  echo "generated $datasource_target"
else
  if [[ "$(read_env_value PRODUCTION_POSTGRES_PASSWORD "$datasource_target")" != "$production_postgres_password" ||
        "$(read_env_value STAGING_POSTGRES_PASSWORD "$datasource_target")" != "$staging_postgres_password" ]]; then
    echo "PostgreSQL observability passwords are out of sync; rerun with --refresh" >&2
    exit 1
  fi
  echo "preserved $datasource_target"
fi

unset postgres_changed production_postgres_password staging_postgres_password

for qdrant_source in "$production_qdrant_target" "$staging_qdrant_target"; do
  if [[ ! -f "$qdrant_source" ]]; then
    echo "missing Qdrant environment file: $qdrant_source" >&2
    exit 1
  fi
done

production_qdrant_key="$(read_env_value HIVY_QDRANT_API_KEY "$production_qdrant_target")"
staging_qdrant_key="$(read_env_value HIVY_QDRANT_API_KEY "$staging_qdrant_target")"
if [[ -z "$production_qdrant_key" || -z "$staging_qdrant_key" ]]; then
  echo "HIVY_QDRANT_API_KEY is required in both environment Qdrant files" >&2
  exit 1
fi

if [[ ! -e "$qdrant_metrics_target" ||
      "$(read_env_value PRODUCTION_QDRANT_API_KEY "$qdrant_metrics_target")" != "$production_qdrant_key" ||
      "$(read_env_value STAGING_QDRANT_API_KEY "$qdrant_metrics_target")" != "$staging_qdrant_key" ]]; then
  printf 'PRODUCTION_QDRANT_API_KEY=%s\nSTAGING_QDRANT_API_KEY=%s\n' \
    "$production_qdrant_key" "$staging_qdrant_key" > "$qdrant_metrics_target"
  chmod 600 "$qdrant_metrics_target"
  echo "synchronized $qdrant_metrics_target"
else
  echo "preserved $qdrant_metrics_target"
fi

unset production_qdrant_key staging_qdrant_key
