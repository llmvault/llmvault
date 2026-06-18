#!/usr/bin/env bash
set -euo pipefail

manifest="${1:?usage: update-railway-runtime-config.sh <release-manifest.json>}"
: "${RAILWAY_TOKEN:?RAILWAY_TOKEN is required}"
: "${RAILWAY_ENVIRONMENT:?RAILWAY_ENVIRONMENT is required}"
: "${RAILWAY_SERVICES:?RAILWAY_SERVICES is required}"

environment="${RAILWAY_ENVIRONMENT}"
services="${RAILWAY_SERVICES}"
runtime_arch_suffix="${HIVY_SANDBOXES_RUNTIME_IMAGE_ARCH_SUFFIX:-amd64}"
wait_seconds="${RAILWAY_DEPLOY_WAIT_SECONDS:-900}"
poll_seconds="${RAILWAY_DEPLOY_POLL_SECONDS:-10}"
railway_attempts=3
railway_attempt_timeout_seconds=120
railway_retry_sleep_seconds=10

if [[ ! -f "${manifest}" ]]; then
  echo "manifest not found: ${manifest}" >&2
  exit 1
fi

command -v jq >/dev/null || {
  echo "jq is required" >&2
  exit 1
}
command -v railway >/dev/null || {
  echo "railway CLI is required" >&2
  exit 1
}

run_with_timeout() {
  local timeout_seconds="${1}"
  shift

  "$@" &
  local pid="${!}"
  local elapsed=0

  while kill -0 "${pid}" 2>/dev/null; do
    if ((elapsed >= timeout_seconds)); then
      kill "${pid}" 2>/dev/null || true
      wait "${pid}" 2>/dev/null || true
      return 124
    fi

    sleep 1
    elapsed=$((elapsed + 1))
  done

  wait "${pid}"
}

railway_with_retry() {
  local attempt
  local exit_code

  for ((attempt = 1; attempt <= railway_attempts; attempt++)); do
    if run_with_timeout "${railway_attempt_timeout_seconds}" railway "$@"; then
      return 0
    else
      exit_code="${?}"
    fi

    if ((attempt == railway_attempts)); then
      return "${exit_code}"
    fi

    echo "Railway command failed with exit ${exit_code}; retrying in ${railway_retry_sleep_seconds}s (${attempt}/${railway_attempts})..." >&2
    sleep "${railway_retry_sleep_seconds}"
  done
}

sandboxes_runtime_image_tag="$(jq -r '.runtimeConfig.HIVY_SANDBOXES_RUNTIME_IMAGE_TAG' "${manifest}")"

if [[ -n "${runtime_arch_suffix}" && "${sandboxes_runtime_image_tag}" != *"-${runtime_arch_suffix}" ]]; then
  sandboxes_runtime_image_tag="${sandboxes_runtime_image_tag}-${runtime_arch_suffix}"
fi

for value in \
  "${sandboxes_runtime_image_tag}"
do
  if [[ -z "${value}" || "${value}" == "null" ]]; then
    echo "release manifest is missing a runtimeConfig value" >&2
    exit 1
  fi
done

read -r -a service_list <<<"${services}"
if [[ "${#service_list[@]}" -eq 0 ]]; then
  echo "no Railway services configured" >&2
  exit 1
fi

for service in "${service_list[@]}"; do
  echo "Updating Railway runtime config on ${service}..."
  railway_with_retry variable set \
    "HIVY_SANDBOXES_RUNTIME_IMAGE_TAG=${sandboxes_runtime_image_tag}" \
    --environment "${environment}" \
    --service "${service}"

  variables="$(
    railway_with_retry variable list \
      --environment "${environment}" \
      --service "${service}" \
      --json
  )"
  for key_and_expected in \
    "HIVY_SANDBOXES_RUNTIME_IMAGE_TAG=${sandboxes_runtime_image_tag}"
  do
    key="${key_and_expected%%=*}"
    expected="${key_and_expected#*=}"
    actual="$(jq -r --arg key "${key}" '.[$key] // ""' <<<"${variables}")"
    if [[ "${actual}" != "${expected}" ]]; then
      echo "Railway variable verification failed for ${service}: ${key}=${actual}, expected ${expected}" >&2
      exit 1
    fi
  done
done

deadline=$((SECONDS + wait_seconds))
while true; do
  all_success=true
  for service in "${service_list[@]}"; do
    status="$(
      railway_with_retry deployment list \
        --environment "${environment}" \
        --service "${service}" \
        --limit 1 \
        --json | jq -r '.[0].status'
    )"
    echo "${service}: ${status}"
    case "${status}" in
      SUCCESS)
        ;;
      FAILED | CRASHED | REMOVED)
        echo "Railway deployment failed for ${service}: ${status}" >&2
        exit 1
        ;;
      *)
        all_success=false
        ;;
    esac
  done

  if [[ "${all_success}" == "true" ]]; then
    echo "All Railway deployments are successful."
    exit 0
  fi

  if ((SECONDS >= deadline)); then
    echo "Timed out waiting for Railway deployments." >&2
    exit 1
  fi

  sleep "${poll_seconds}"
done
