#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
target="$repo_root/kubernetes/observability/secrets/grafana-admin.env"

if [[ -e "$target" && "${1:-}" != "--refresh" ]]; then
  echo "refusing to overwrite $target; pass --refresh to rotate it" >&2
  exit 1
fi

umask 077
mkdir -p "$(dirname "$target")"
password="$(openssl rand -base64 48 | tr -d '\n')"
printf 'admin-user=admin\nadmin-password=%s\n' "$password" > "$target"
chmod 600 "$target"
echo "generated $target"
