#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
config_root="${repo_root}/kubernetes/config"
expected_project_id="55776e03-e6c2-4a9b-828b-4e759495aa70"
refresh="false"

if [[ "${1:-}" == "--refresh" ]]; then
  refresh="true"
elif [[ $# -gt 0 ]]; then
  echo "usage: $0 [--refresh]" >&2
  exit 1
fi

for command_name in railway jq openssl; do
  if ! command -v "${command_name}" >/dev/null; then
    echo "missing required command: ${command_name}" >&2
    exit 1
  fi
done

project_id="$(railway status --json | jq -er '.id')"
if [[ "${project_id}" != "${expected_project_id}" ]]; then
  echo "Railway is linked to unexpected project ${project_id}" >&2
  exit 1
fi

web_vars="$(railway variable list --service web.usehivy.com --environment production --json)"

read_required() {
  local key="$1"
  jq -er --arg key "${key}" '.[$key] | select(type == "string" and length > 0)' <<<"${web_vars}"
}

read_optional() {
  local key="$1"
  jq -r --arg key "${key}" '.[$key] // empty' <<<"${web_vars}"
}

write_secret() {
  local environment_name="$1"
  local session_secret="$2"
  local destination="${config_root}/env/${environment_name}/web.env"
  local temp_file
  temp_file="$(mktemp)"
  chmod 600 "${temp_file}"

  if [[ -e "${destination}" && "${refresh}" != "true" ]]; then
    echo "refusing to overwrite ${destination}; pass --refresh" >&2
    rm -f "${temp_file}"
    exit 1
  fi

  printf 'HIVY_SESSION_SECRET=%s\n' "${session_secret}" >>"${temp_file}"
  for key in HIVY_SENTRY_DSN NEXT_PUBLIC_HIVY_SENTRY_DSN; do
    value="$(read_optional "${key}")"
    if [[ -n "${value}" ]]; then
      printf '%s=%s\n' "${key}" "${value}" >>"${temp_file}"
    fi
  done

  mv "${temp_file}" "${destination}"
  chmod 600 "${destination}"
  echo "generated ${environment_name} web secrets"
}

umask 077
write_secret production "$(read_required HIVY_SESSION_SECRET)"
write_secret staging "$(openssl rand -hex 32)"

unset web_vars value
echo "web secrets are ready"
