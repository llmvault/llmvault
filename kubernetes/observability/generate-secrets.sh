#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
secret_dir="$repo_root/kubernetes/config/env/observability"
grafana_target="$secret_dir/grafana-admin.env"
telemetry_target="$secret_dir/telemetry-ingest.env"
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
