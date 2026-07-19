#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
kubeconfig="${KUBECONFIG:-${repo_root}/ansible/.secrets/k8s0/kubeconfig-local.yaml}"
s3_env="${repo_root}/.env.hetzner-s3"
local_env="${repo_root}/.env"
extra_env="/tmp/hivy-backend-new.env"
expected_project_id="55776e03-e6c2-4a9b-828b-4e759495aa70"
refresh="false"

if [[ "${1:-}" == "--refresh" ]]; then
  refresh="true"
elif [[ $# -gt 0 ]]; then
  echo "usage: $0 [--refresh]" >&2
  exit 1
fi

for command_name in railway jq kubectl awk openssl; do
  if ! command -v "${command_name}" >/dev/null; then
    echo "missing required command: ${command_name}" >&2
    exit 1
  fi
done

if [[ ! -f "${kubeconfig}" || ! -f "${s3_env}" ]]; then
  echo "missing kubeconfig or .env.hetzner-s3" >&2
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

microsandbox_token="$(kubectl --kubeconfig "${kubeconfig}" -n production get secret microsandbox-control-secrets -o jsonpath='{.data.HIVY_MICROSANDBOX_API_TOKEN}' | base64 -d)"
preview_activity_token=""
if [[ -f "${local_env}" ]]; then
  preview_activity_token="$(read_env_value "${local_env}" HIVY_PREVIEW_ACTIVITY_TOKEN 2>/dev/null || true)"
fi
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

write_environment() {
  local environment_name="$1"
  local s3_prefix="$2"
  local paystack_key="$3"
  local destination="${repo_root}/kubernetes/environments/${environment_name}/secrets/backend.env"
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
  printf '%s=%s\n' HIVY_PAYSTACK_SECRET_KEY "${paystack_key}" >>"${temp_file}"
  printf '%s=%s\n' HIVY_AWS_ACCESS_KEY_ID "$(read_env_value "${s3_env}" "${s3_prefix}_ACCESS_KEY_ID")" >>"${temp_file}"
  printf '%s=%s\n' HIVY_AWS_SECRET_ACCESS_KEY "$(read_env_value "${s3_env}" "${s3_prefix}_SECRET_ACCESS_KEY")" >>"${temp_file}"

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
  echo "generated ${environment_name} backend secrets"
}

production_paystack="$(read_json_value "${api_vars}" HIVY_PAYSTACK_SECRET_KEY)"
if [[ "${production_paystack}" != sk_live_* ]]; then
  echo "Railway production Paystack key is not live" >&2
  exit 1
fi

if [[ ! -f "${local_env}" ]]; then
  echo "missing ${local_env} containing the staging Paystack test key" >&2
  exit 1
fi
staging_paystack="$(read_env_value "${local_env}" HIVY_PAYSTACK_SECRET_KEY)"
if [[ "${staging_paystack}" != sk_test_* ]]; then
  echo "staging Paystack key is not a test key" >&2
  exit 1
fi

umask 077
write_environment staging HETZNER_S3_STAGING_APP "${staging_paystack}"
write_environment production HETZNER_S3_PROD_APP "${production_paystack}"

unset api_vars worker_vars microsandbox_token preview_activity_token production_paystack staging_paystack value
echo "backend secrets are ready"
