#!/bin/sh
set -eu

backup_root="${REDIS_BACKUP_ROOT:-/backup}"
validation_root="${backup_root}/.validation"
validation_port=6380
active_pid_file=

log() {
  printf '%s\n' "$*"
}

stop_validation_server() {
  if [ -z "${active_pid_file}" ] || [ ! -f "${active_pid_file}" ]; then
    return
  fi

  redis-cli -h 127.0.0.1 -p "${validation_port}" shutdown nosave >/dev/null 2>&1 || true
  pid="$(cat "${active_pid_file}" 2>/dev/null || true)"
  if [ -z "${pid}" ]; then
    active_pid_file=
    return
  fi
  attempt=0
  while kill -0 "${pid}" >/dev/null 2>&1 && [ "${attempt}" -lt 10 ]; do
    sleep 1
    attempt=$((attempt + 1))
  done
  if kill -0 "${pid}" >/dev/null 2>&1; then
    kill "${pid}" >/dev/null 2>&1 || true
  fi
  active_pid_file=
}

trap stop_validation_server EXIT HUP INT TERM

prepare_rdb_checker() {
  mkdir -p "${validation_root}"
  checker="${validation_root}/redis-check-rdb"
  if [ ! -e "${checker}" ]; then
    ln -s /usr/local/bin/redis-server "${checker}"
  fi
  printf '%s\n' "${checker}"
}

validate_rdb() {
  rdb_path="$1"
  label="$2"
  checker="$3"
  check_log="${validation_root}/${label}-check.log"
  redis_log="${validation_root}/${label}-redis.log"
  active_pid_file="${validation_root}/${label}.pid"

  if [ ! -s "${rdb_path}" ]; then
    log "Redis backup is empty: ${label}."
    return 1
  fi

  if ! "${checker}" "${rdb_path}" >"${check_log}" 2>&1; then
    log "Redis RDB integrity check failed: ${label}."
    cat "${check_log}"
    return 1
  fi

  rm -f "${active_pid_file}" "${redis_log}"
  redis-server \
    --bind 127.0.0.1 \
    --port "${validation_port}" \
    --dir "$(dirname "${rdb_path}")" \
    --dbfilename "$(basename "${rdb_path}")" \
    --appendonly no \
    --save "" \
    --requirepass "${REDIS_PASSWORD}" \
    --daemonize yes \
    --pidfile "${active_pid_file}" \
    --logfile "${redis_log}"

  ready=false
  attempt=0
  while [ "${attempt}" -lt 60 ]; do
    if redis-cli -h 127.0.0.1 -p "${validation_port}" ping 2>/dev/null | grep -qx PONG; then
      ready=true
      break
    fi
    if [ -f "${active_pid_file}" ]; then
      pid="$(cat "${active_pid_file}")"
      if ! kill -0 "${pid}" >/dev/null 2>&1; then
        break
      fi
    fi
    sleep 1
    attempt=$((attempt + 1))
  done

  if [ "${ready}" != true ]; then
    log "Redis RDB boot validation failed: ${label}."
    cat "${redis_log}"
    stop_validation_server
    return 1
  fi

  key_count="$(redis-cli -h 127.0.0.1 -p "${validation_port}" dbsize)"
  stop_validation_server
  log "Validated Redis backup ${label} (${key_count} keys)."
}

capture_cluster_state() {
  output_prefix="$1"
  host=backend-redis-leader-0.backend-redis-leader-headless

  redis-cli -h "${host}" -p 6379 cluster info |
    tr -d '\r' >"${output_prefix}-info.txt"
  redis-cli -h "${host}" -p 6379 cluster nodes |
    tr -d '\r' >"${output_prefix}-nodes.txt"

  grep -qx 'cluster_state:ok' "${output_prefix}-info.txt"
  grep -qx 'cluster_slots_assigned:16384' "${output_prefix}-info.txt"
  master_count="$(
    awk '$3 ~ /(^|,)master(,|$)/ { count++ } END { print count + 0 }' \
      "${output_prefix}-nodes.txt"
  )"
  if [ "${master_count}" -ne 3 ]; then
    log "Expected three Redis cluster masters, found ${master_count}."
    return 1
  fi
  if awk '$3 ~ /fail/ || $8 != "connected" { unhealthy = 1 }
    END { exit unhealthy ? 0 : 1 }' \
    "${output_prefix}-nodes.txt"; then
    log "Redis cluster contains a failed or disconnected node."
    return 1
  fi

  awk '{
    printf "%s %s %s %s", $1, $3, $4, $8
    for (field = 9; field <= NF; field++) {
      printf " %s", $field
    }
    printf "\n"
  }' "${output_prefix}-nodes.txt" |
    sort >"${output_prefix}-topology.txt"
}

backup_cluster() {
  backup_dir="$1"
  capture_cluster_state "${validation_root}/before"

  for ordinal in 0 1 2; do
    host="backend-redis-leader-${ordinal}.backend-redis-leader-headless"
    redis-cli -h "${host}" -p 6379 \
      --rdb "${backup_dir}/leader-${ordinal}.rdb"
  done

  capture_cluster_state "${validation_root}/after"
  if ! cmp -s \
    "${validation_root}/before-topology.txt" \
    "${validation_root}/after-topology.txt"; then
    log "Redis cluster topology changed while snapshots were exported."
    diff -u \
      "${validation_root}/before-topology.txt" \
      "${validation_root}/after-topology.txt" || true
    return 1
  fi

  cp "${validation_root}/after-nodes.txt" "${backup_dir}/cluster-nodes.txt"
  cp "${validation_root}/after-info.txt" "${backup_dir}/cluster-info.txt"

  checker="$(prepare_rdb_checker)"
  for ordinal in 0 1 2; do
    validate_rdb "${backup_dir}/leader-${ordinal}.rdb" "leader-${ordinal}" "${checker}"
  done

  (
    cd "${backup_dir}"
    sha256sum \
      leader-0.rdb \
      leader-1.rdb \
      leader-2.rdb \
      cluster-nodes.txt \
      cluster-info.txt >manifest.sha256
  )
}

backup_standalone() {
  backup_dir="$1"
  redis-cli -h backend-redis -p 6379 --rdb "${backup_dir}/standalone.rdb"

  checker="$(prepare_rdb_checker)"
  validate_rdb "${backup_dir}/standalone.rdb" standalone "${checker}"

  (
    cd "${backup_dir}"
    sha256sum standalone.rdb >manifest.sha256
  )
}

main() {
  export REDISCLI_AUTH="${REDIS_PASSWORD}"
  backup_time="$(date -u +%Y%m%dT%H%M%SZ)"
  backup_dir="${backup_root}/${backup_time}"
  mkdir -p "${backup_dir}" "${validation_root}"

  case "${REDIS_BACKUP_MODE}" in
    cluster)
      backup_cluster "${backup_dir}"
      ;;
    standalone)
      backup_standalone "${backup_dir}"
      ;;
    *)
      log "Unsupported Redis backup mode: ${REDIS_BACKUP_MODE}."
      return 1
      ;;
  esac

  printf '%s\n' "${backup_time}" >"${backup_root}/latest-backup-time"
  log "Exported and validated Redis backup ${backup_time}."
}

main "$@"
