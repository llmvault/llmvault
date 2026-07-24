#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
target="$repo_root/kubernetes/config/env/ansible/runners.env"
key="HIVY_MICROSANDBOX_LOG_INGEST_SIGNING_KEY"

if [[ ! -f "$target" ]]; then
  echo "missing $target; create it from runners.env.example first" >&2
  exit 1
fi

if grep -Eq "^${key}=.{32,}$" "$target"; then
  echo "preserved existing sandbox log signing key"
  exit 0
fi

if grep -Eq "^${key}=" "$target"; then
  value="$(openssl rand -hex 48)"
  awk -v key="$key" -v value="$value" 'BEGIN { FS=OFS="=" } $1 == key { print key, value; next } { print }' "$target" > "${target}.tmp"
  chmod --reference="$target" "${target}.tmp"
  mv "${target}.tmp" "$target"
else
  printf '\n%s=%s\n' "$key" "$(openssl rand -hex 48)" >> "$target"
fi

chmod 600 "$target"
echo "configured sandbox log signing key"
