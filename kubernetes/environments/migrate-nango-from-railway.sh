#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
kubeconfig="${KUBECONFIG:-${repo_root}/ansible/.secrets/k8s0/kubeconfig-local.yaml}"
expected_project_id="55776e03-e6c2-4a9b-828b-4e759495aa70"
mkdir -p "${repo_root}/tmp"
work_dir="$(mktemp -d "${repo_root}/tmp/nango-migrate.XXXXXX")"
dump_path="${work_dir}/nango.dump"

cleanup() {
  rm -rf "${work_dir}"
}
trap cleanup EXIT

for command_name in railway jq docker kubectl; do
  if ! command -v "${command_name}" >/dev/null; then
    echo "missing required command: ${command_name}" >&2
    exit 1
  fi
done

if [[ ! -f "${kubeconfig}" ]]; then
  echo "missing kubeconfig: ${kubeconfig}" >&2
  exit 1
fi

project_id="$(railway status --json | jq -er '.id')"
if [[ "${project_id}" != "${expected_project_id}" ]]; then
  echo "Railway is linked to unexpected project ${project_id}" >&2
  exit 1
fi

source_url="$(railway variable list --service db.integrations.usehivy.com --environment production --json | jq -er '.DATABASE_PUBLIC_URL')"

echo "dumping Railway Nango PostgreSQL"
DATABASE_URL="${source_url}" docker run --rm \
  --env DATABASE_URL \
  --volume "${work_dir}:/dump" \
  postgres:17-alpine \
  sh -c 'exec pg_dump --dbname="$DATABASE_URL" --format=custom --no-owner --no-privileges --file=/dump/nango.dump'
unset source_url

if [[ ! -s "${dump_path}" ]]; then
  echo "Railway dump is empty" >&2
  exit 1
fi

restore_environment() {
  local namespace="$1"
  local existing_replicas="0"

  kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" wait \
    --for=condition=Ready cluster.postgresql.cnpg.io/nango-postgres --timeout=15m

  if kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" get deployment nango >/dev/null 2>&1; then
    existing_replicas="$(kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" get deployment nango -o jsonpath='{.spec.replicas}')"
    kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" scale deployment/nango --replicas=0 >/dev/null
    kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" wait \
      --for=delete pod -l app.kubernetes.io/name=nango --timeout=5m
  fi

  local primary
  local password
  primary="$(kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" get pod \
    -l cnpg.io/cluster=nango-postgres,cnpg.io/instanceRole=primary \
    -o jsonpath='{.items[0].metadata.name}')"
  password="$(kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" get secret nango-postgres-app \
    -o jsonpath='{.data.password}' | base64 -d)"

  echo "restoring Nango into ${namespace}"
  kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" exec -i "${primary}" -c postgres -- \
    env PGPASSWORD="${password}" pg_restore \
      --host=127.0.0.1 \
      --username=nango \
      --dbname=nango \
      --clean \
      --if-exists \
      --exit-on-error \
      --no-owner \
      --no-privileges \
    <"${dump_path}"

  kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" exec "${primary}" -c postgres -- \
    env PGPASSWORD="${password}" psql \
      --host=127.0.0.1 \
      --username=nango \
      --dbname=nango \
      --set=ON_ERROR_STOP=1 \
      --command="update nango._nango_external_webhooks set primary_url = 'http://backend-api:8080/internal/webhooks/nango' where nullif(trim(primary_url), '') is not null;"

  kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" exec "${primary}" -c postgres -- \
    env PGPASSWORD="${password}" psql \
      --host=127.0.0.1 \
      --username=nango \
      --dbname=nango \
      --tuples-only \
      --no-align \
      --command="select 'configs=' || count(*) from nango._nango_configs; select 'connections=' || count(*) from nango._nango_connections;"
  unset password

  if [[ "${existing_replicas}" != "0" ]]; then
    kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" scale deployment/nango --replicas="${existing_replicas}" >/dev/null
    kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" rollout status deployment/nango --timeout=10m
  fi
}

restore_environment staging
restore_environment production

echo "Railway Nango database restored into staging and production"
