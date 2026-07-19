#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
kubeconfig="${KUBECONFIG:-${repo_root}/ansible/.secrets/k8s0/kubeconfig-local.yaml}"
expected_project_id="55776e03-e6c2-4a9b-828b-4e759495aa70"
mkdir -p "${repo_root}/tmp"
work_dir="$(mktemp -d "${repo_root}/tmp/backend-migrate.XXXXXX")"
dump_path="${work_dir}/hivy.dump"
restore_list="${work_dir}/restore.list"

cleanup() {
  rm -rf "${work_dir}"
}
trap cleanup EXIT

for command_name in railway jq docker kubectl awk; do
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

source_url="$(railway variable list --service api.postgres.usehivy.com --environment production --json | jq -er '.DATABASE_PUBLIC_URL')"

echo "dumping Railway backend PostgreSQL"
dump_complete="false"
for attempt in 1 2 3; do
  rm -f "${dump_path}"
  if DATABASE_URL="${source_url}" docker run --rm \
    --env DATABASE_URL \
    --volume "${work_dir}:/dump" \
    postgres:17-alpine \
    sh -c 'exec pg_dump --dbname="$DATABASE_URL" --format=custom --no-owner --no-privileges --file=/dump/hivy.dump'; then
    dump_complete="true"
    break
  fi
  echo "Railway dump attempt ${attempt} failed" >&2
done
unset source_url

if [[ "${dump_complete}" != "true" || ! -s "${dump_path}" ]]; then
  echo "Railway dump did not complete" >&2
  exit 1
fi

docker run --rm --volume "${work_dir}:/dump" postgres:17-alpine \
  sh -c 'exec pg_restore --list /dump/hivy.dump' \
  | awk '!/ EXTENSION - / && !/ COMMENT - EXTENSION /' >"${restore_list}"

restore_environment() {
  local namespace="$1"
  local primary
  local password
  # CloudNativePG mounts a writable ephemeral scratch volume at /run while the
  # container root filesystem and /tmp are read-only.
  local remote_dir="/run/hivy-backend-migration"

  kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" wait \
    --for=condition=Ready cluster.postgresql.cnpg.io/backend-postgres --timeout=15m

  for deployment_name in backend-api backend-worker; do
    if kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" get deployment "${deployment_name}" >/dev/null 2>&1; then
      kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" scale deployment "${deployment_name}" --replicas=0 >/dev/null
    fi
  done
  kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" wait \
    --for=delete pod -l app.kubernetes.io/name=hivy-backend --timeout=5m >/dev/null 2>&1 || true

  primary="$(kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" get pod \
    -l cnpg.io/cluster=backend-postgres,role=primary \
    -o jsonpath='{.items[0].metadata.name}')"
  password="$(kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" get secret backend-postgres-app \
    -o jsonpath='{.data.password}' | base64 -d)"

  echo "restoring backend into ${namespace}"
  kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" exec "${primary}" -c postgres -- \
    sh -c "mkdir -p '${remote_dir}' && chmod 700 '${remote_dir}'"
  kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" cp "${dump_path}" "${primary}:${remote_dir}/hivy.dump" -c postgres
  kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" cp "${restore_list}" "${primary}:${remote_dir}/restore.list" -c postgres
  printf '127.0.0.1:5432:hivy:hivy:%s\n' "${password}" | \
    kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" exec -i "${primary}" -c postgres -- \
      sh -c "umask 077; cat > '${remote_dir}/pgpass'"
  unset password

  kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" exec "${primary}" -c postgres -- \
    psql --dbname=hivy --set=ON_ERROR_STOP=1 \
      --command='create extension if not exists pg_trgm; create extension if not exists pgcrypto; create extension if not exists vector;'

  kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" exec "${primary}" -c postgres -- \
    sh -ec "PGPASSFILE='${remote_dir}/pgpass' exec pg_restore \
      --host=127.0.0.1 \
      --username=hivy \
      --dbname=hivy \
      --clean \
      --if-exists \
      --exit-on-error \
      --no-owner \
      --no-privileges \
      --use-list='${remote_dir}/restore.list' \
      '${remote_dir}/hivy.dump'"

  kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" exec "${primary}" -c postgres -- \
    psql --dbname=hivy --tuples-only --no-align \
      --command="select 'tables=' || count(*) from pg_catalog.pg_tables where schemaname not in ('pg_catalog','information_schema'); select 'users=' || count(*) from users; select 'goose_version=' || coalesce(max(version_id) filter (where is_applied),0) from goose_db_version;"

  kubectl --kubeconfig "${kubeconfig}" -n "${namespace}" exec "${primary}" -c postgres -- \
    sh -c "rm -f '${remote_dir}/hivy.dump' '${remote_dir}/restore.list' '${remote_dir}/pgpass' && rmdir '${remote_dir}'"
}

restore_environment staging
restore_environment production

echo "Railway backend database restored into staging and production"
