#!/usr/bin/env bash
set -euo pipefail

secret_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
qdrant_env="${secret_dir}/qdrant.env"

if [[ -e "${qdrant_env}" ]]; then
  echo "refusing to overwrite existing secret file: ${qdrant_env}" >&2
  exit 1
fi

umask 077
qdrant_api_key="$(openssl rand -hex 32)"

printf '%s\n' \
  "HIVY_QDRANT_API_KEY=${qdrant_api_key}" \
  > "${qdrant_env}"

chmod 600 "${qdrant_env}"
echo "generated staging Qdrant secret input in ${qdrant_env}"
