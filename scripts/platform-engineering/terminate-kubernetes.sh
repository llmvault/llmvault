#!/usr/bin/env bash
set -euo pipefail

state_dir_input="${K8S_TUNNEL_STATE_DIR:-${TMPDIR:-/tmp}/hivy-platform-engineering-agent}"
case "${state_dir_input}" in
  ""|/|/tmp|"${HOME:-__unset_home__}")
    echo "refusing unsafe K8S_TUNNEL_STATE_DIR: ${state_dir_input}" >&2
    exit 1
    ;;
esac

if [[ ! -d "${state_dir_input}" ]]; then
  echo "Kubernetes tunnel is already stopped."
  exit 0
fi

state_dir="$(cd "${state_dir_input}" && pwd -P)"
control_socket="${state_dir}/ssh-control"
connected_host_file="${state_dir}/connected-host"
connected_user_file="${state_dir}/connected-user"

if [[ -f "${connected_host_file}" && -f "${connected_user_file}" ]]; then
  connected_host="$(<"${connected_host_file}")"
  connected_user="$(<"${connected_user_file}")"
  ssh -S "${control_socket}" -O exit \
    "${connected_user}@${connected_host}" >/dev/null 2>&1 || true
fi

rm -f -- \
  "${state_dir}/connected-host" \
  "${state_dir}/connected-user" \
  "${state_dir}/known-hosts" \
  "${state_dir}/kubeconfig" \
  "${state_dir}/ssh-control" \
  "${state_dir}/tunnel-key"
rmdir "${state_dir}" 2>/dev/null || true

echo "Kubernetes tunnel stopped and local session credentials removed."
