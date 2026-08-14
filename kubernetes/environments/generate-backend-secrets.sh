#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
config_root="${repo_root}/kubernetes/config"
s3_env="${config_root}/env/infrastructure/hetzner-s3.env"
microsandbox_env="${config_root}/env/production/microsandbox-control.env"
extra_env="${config_root}/env/infrastructure/backend-overrides.env"
expected_project_id="55776e03-e6c2-4a9b-828b-4e759495aa70"
refresh="false"

if [[ "${1:-}" == "--refresh" ]]; then
  refresh="true"
elif [[ $# -gt 0 ]]; then
  echo "usage: $0 [--refresh]" >&2
  exit 1
fi

for command_name in railway jq awk openssl; do
  if ! command -v "${command_name}" >/dev/null; then
    echo "missing required command: ${command_name}" >&2
    exit 1
  fi
done

if [[ ! -f "${s3_env}" || ! -f "${microsandbox_env}" ]]; then
  echo "missing Hetzner S3 or production Microsandbox environment file" >&2
  exit 1
fi

project_id="$(railway status --json | jq -er '.id')"
if [[ "${project_id}" != "${expected_project_id}" ]]; then
  echo "Railway is linked to unexpected project ${project_id}" >&2
  exit 1
fi

api_vars="$(railway variable list --service api.usehivy.com --environment production --json)"
worker_vars="$(railway variable list --service asynq.usehivy.com --environment production --json)"

read_json_value() {
  local document="$1"
  local key="$2"
  jq -er --arg key "${key}" '.[$key] | select(type == "string" and length > 0)' <<<"${document}"
}

read_env_value() {
  local file="$1"
  local key="$2"
  awk -v key="${key}" 'index($0, key "=") == 1 { print substr($0, length(key) + 2); found = 1 } END { if (!found) exit 1 }' "${file}"
}

for key in \
  HIVY_KMS_KEY \
  HIVY_JWT_SIGNING_KEY \
  HIVY_AUTH_RSA_PRIVATE_KEY \
  HIVY_SANDBOX_ENCRYPTION_KEY \
  HIVY_MICROSANDBOX_CONTROL_API_TOKEN \
  HIVY_NANGO_SECRET_KEY \
  HIVY_NANGO_WEBHOOKS_SECRET; do
  api_value="$(read_json_value "${api_vars}" "${key}")"
  worker_value="$(read_json_value "${worker_vars}" "${key}")"
  if [[ "${api_value}" != "${worker_value}" ]]; then
    echo "Railway API and worker disagree on ${key}" >&2
    exit 1
  fi
done
unset api_value worker_value

microsandbox_token="$(read_env_value "${microsandbox_env}" HIVY_MICROSANDBOX_API_TOKEN)"
preview_activity_token="$(jq -r '.HIVY_PREVIEW_ACTIVITY_TOKEN // empty' <<<"${api_vars}")"
if [[ -z "${preview_activity_token}" ]]; then
  preview_activity_token="$(openssl rand -hex 32)"
fi

railway_secret_keys=(
  HIVY_KMS_KEY
  HIVY_JWT_SIGNING_KEY
  HIVY_AUTH_RSA_PRIVATE_KEY
  HIVY_SANDBOX_ENCRYPTION_KEY
  HIVY_ADMIN_SECRET
  HIVY_SMTP_PASSWORD
  HIVY_OAUTH_GITHUB_CLIENT_ID
  HIVY_OAUTH_GITHUB_CLIENT_SECRET
  HIVY_OAUTH_GOOGLE_CLIENT_ID
  HIVY_OAUTH_GOOGLE_CLIENT_SECRET
  HIVY_OAUTH_X_CLIENT_ID
  HIVY_OAUTH_X_CLIENT_SECRET
  HIVY_SPIDER_CLOUD_API_KEY
  HIVY_FIRECRAWL_API_KEY
  HIVY_SERPER_API_KEY
  HIVY_SENTRY_DSN
  HIVY_AGENT_SANDBOX_SENTRY_DSN
  HIVY_LLM_API_KEY
  HIVY_RERANKER_API_KEY
  HIVY_GITHUB_TOKEN
)

write_production() {
  local destination="${config_root}/env/production/backend.env"
  local temp_file
  temp_file="$(mktemp)"
  chmod 600 "${temp_file}"

  if [[ -e "${destination}" && "${refresh}" != "true" ]]; then
    echo "refusing to overwrite ${destination}; pass --refresh" >&2
    rm -f "${temp_file}"
    exit 1
  fi

  for key in "${railway_secret_keys[@]}"; do
    value="$(read_json_value "${api_vars}" "${key}")"
    printf '%s=%s\n' "${key}" "${value}" >>"${temp_file}"
  done

  printf '%s=%s\n' HIVY_MICROSANDBOX_CONTROL_API_TOKEN "${microsandbox_token}" >>"${temp_file}"
  printf '%s=%s\n' HIVY_PREVIEW_ACTIVITY_TOKEN "${preview_activity_token}" >>"${temp_file}"
  printf '%s=%s\n' HIVY_PAYSTACK_SECRET_KEY "${production_paystack}" >>"${temp_file}"
  printf '%s=%s\n' HIVY_AWS_ACCESS_KEY_ID "$(read_env_value "${s3_env}" HETZNER_S3_PROD_APP_ACCESS_KEY_ID)" >>"${temp_file}"
  printf '%s=%s\n' HIVY_AWS_SECRET_ACCESS_KEY "$(read_env_value "${s3_env}" HETZNER_S3_PROD_APP_SECRET_ACCESS_KEY)" >>"${temp_file}"

  if [[ -f "${extra_env}" ]]; then
    while IFS= read -r line; do
      [[ -z "${line}" || "${line}" == \#* ]] && continue
      if [[ ! "${line}" =~ ^HIVY_[A-Z0-9_]+=.+$ ]]; then
        echo "invalid line in ${extra_env}" >&2
        rm -f "${temp_file}"
        exit 1
      fi
      key="${line%%=*}"
      if awk -F= -v key="${key}" '$1 == key { found = 1 } END { exit !found }' "${temp_file}"; then
        echo "${extra_env} attempts to override managed key ${key}" >&2
        rm -f "${temp_file}"
        exit 1
      fi
      printf '%s\n' "${line}" >>"${temp_file}"
    done <"${extra_env}"
  fi

  mv "${temp_file}" "${destination}"
  chmod 600 "${destination}"
  echo "generated production backend secrets"
}

production_paystack="$(read_json_value "${api_vars}" HIVY_PAYSTACK_SECRET_KEY)"
if [[ "${production_paystack}" != sk_live_* ]]; then
  echo "Railway production Paystack key is not live" >&2
  exit 1
fi

umask 077
write_production

unset api_vars worker_vars microsandbox_token preview_activity_token production_paystack value
echo "production backend secrets are ready"
