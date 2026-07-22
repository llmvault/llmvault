#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
config_root="${repo_root}/kubernetes/config"

required_env_files=(
  env/ansible/runners.env
  env/infrastructure/backend-overrides.env
  env/infrastructure/hetzner-s3.env
  env/observability/grafana-admin.env
  env/platform-engineering/platform-engineering-agent.env
  env/production/backend.env
  env/production/microsandbox-control.env
  env/production/microsandbox-postgres-backup.env
  env/production/microsandbox-postgres.env
  env/production/nango-backend.env
  env/production/nango-postgres-backup.env
  env/production/nango-postgres.env
  env/production/nango-runtime.env
  env/production/postgres-backup.env
  env/production/postgres.env
  env/production/qdrant-backup.env
  env/production/qdrant.env
  env/production/redis-backup.env
  env/production/redis.env
  env/production/web.env
  env/staging/backend.env
  env/staging/nango-backend.env
  env/staging/nango-postgres-backup.env
  env/staging/nango-postgres.env
  env/staging/nango-runtime.env
  env/staging/postgres-backup.env
  env/staging/postgres.env
  env/staging/qdrant-backup.env
  env/staging/qdrant.env
  env/staging/redis-backup.env
  env/staging/redis.env
  env/staging/web.env
)

required_kubeconfigs=(
  kubeconfigs/github-actions/production.yaml
  kubeconfigs/github-actions/staging.yaml
  kubeconfigs/k8s0/admin.yaml
  kubeconfigs/k8s0/local.yaml
  kubeconfigs/k8s0/tunnel.yaml
  kubeconfigs/k8s1/admin.yaml
  kubeconfigs/platform-engineering-agent.yaml
)

required_credentials=(
  credentials/github-actions/production
  credentials/github-actions/production.pub
  credentials/github-actions/staging
  credentials/github-actions/staging.pub
  credentials/k3s/k8s0/cluster-secrets.yaml
  credentials/k3s/k8s0/k3s-etcd-s3.yaml
  credentials/k3s/k8s0/node-token
  credentials/k3s/k8s0/server-token
  credentials/k3s/k8s1/node-token
  credentials/k3s/k8s1/server-token
  credentials/providers/hetzner-token
  credentials/providers/vercel-token
  credentials/platform-engineering-agent/known_hosts
  credentials/platform-engineering-agent/tunnel
  credentials/platform-engineering-agent/tunnel.pub
)

tracked_config_env_files=(
  env/production/backend.config.env
  env/production/microsandbox-control.config.env
  env/production/nango.config.env
  env/production/web.config.env
  env/staging/backend.config.env
  env/staging/nango.config.env
  env/staging/web.config.env
  env/platform-engineering/platform-engineering-agent.config.env
)

file_mode() {
  if stat -f '%Lp' "$1" >/dev/null 2>&1; then
    stat -f '%Lp' "$1"
  else
    stat -c '%a' "$1"
  fi
}

validate_private_file() {
  local relative_path="$1"
  local absolute_path="${config_root}/${relative_path}"
  local mode

  if [[ ! -f "${absolute_path}" ]]; then
    echo "missing infrastructure file: kubernetes/config/${relative_path}" >&2
    return 1
  fi
  if ! git -C "${repo_root}" check-ignore -q -- "${absolute_path}"; then
    echo "infrastructure file is not Git-ignored: kubernetes/config/${relative_path}" >&2
    return 1
  fi
  if git -C "${repo_root}" ls-files --error-unmatch -- "${absolute_path}" >/dev/null 2>&1; then
    echo "infrastructure file is tracked by Git: kubernetes/config/${relative_path}" >&2
    return 1
  fi

  mode="$(file_mode "${absolute_path}")"
  if (( (8#${mode} & 077) != 0 )); then
    echo "infrastructure file permissions are too broad (${mode}): kubernetes/config/${relative_path}" >&2
    return 1
  fi
}

validate_env_syntax() {
  local absolute_path="$1"
  local display_path="$2"
  if ! awk '
    /^[[:space:]]*($|#)/ { next }
    !/^(export[[:space:]]+)?[A-Za-z_][A-Za-z0-9_.-]*=/ { exit 1 }
  ' "${absolute_path}"; then
    echo "invalid environment-file syntax: ${display_path}" >&2
    return 1
  fi
  if ! awk -F= '
    /^[[:space:]]*($|#)/ { next }
    {
      key = $1
      sub(/^export[[:space:]]+/, "", key)
      if (seen[key]++) exit 1
    }
  ' "${absolute_path}"; then
    echo "duplicate environment key: ${display_path}" >&2
    return 1
  fi
}

validate_env_file() {
  local relative_path="$1"
  local absolute_path="${config_root}/${relative_path}"

  validate_private_file "${relative_path}"
  validate_env_syntax "${absolute_path}" "kubernetes/config/${relative_path}"
}

for relative_path in "${required_env_files[@]}"; do
  validate_env_file "${relative_path}"
done
for relative_path in "${required_kubeconfigs[@]}" "${required_credentials[@]}"; do
  validate_private_file "${relative_path}"
done
for relative_path in "${tracked_config_env_files[@]}"; do
  absolute_path="${config_root}/${relative_path}"
  if [[ ! -f "${absolute_path}" ]]; then
    echo "missing service configuration: kubernetes/config/${relative_path}" >&2
    exit 1
  fi
  if git -C "${repo_root}" check-ignore -q -- "${absolute_path}"; then
    echo "service configuration is unexpectedly Git-ignored: kubernetes/config/${relative_path}" >&2
    exit 1
  fi
  validate_env_syntax "${absolute_path}" "kubernetes/config/${relative_path}"
done

legacy_paths=(
  "${repo_root}/.env.hetzner-s3"
  "${repo_root}/ansible/.env"
  "${repo_root}/ansible/.secrets"
  "${repo_root}/kubernetes/environments/production/secrets"
  "${repo_root}/kubernetes/environments/staging/secrets"
  "${repo_root}/kubernetes/observability/secrets"
)
for legacy_path in "${legacy_paths[@]}"; do
  if [[ -e "${legacy_path}" ]]; then
    echo "legacy infrastructure path still exists: ${legacy_path#${repo_root}/}" >&2
    exit 1
  fi
done

echo "centralized infrastructure configuration is complete and Git-ignored"
