#!/usr/bin/env bash
set -euo pipefail

: "${KUBE_CONFIG_B64:?KUBE_CONFIG_B64 is required}"
: "${K8S_TUNNEL_SSH_KEY_B64:?K8S_TUNNEL_SSH_KEY_B64 is required}"
: "${K8S_TUNNEL_KNOWN_HOSTS_B64:?K8S_TUNNEL_KNOWN_HOSTS_B64 is required}"
: "${K8S_TUNNEL_HOSTS:?K8S_TUNNEL_HOSTS is required}"
: "${K8S_TUNNEL_USER:?K8S_TUNNEL_USER is required}"
: "${RUNNER_TEMP:?RUNNER_TEMP is required}"

export KUBECONFIG="${KUBECONFIG:-${RUNNER_TEMP}/hivy-kubeconfig}"
ssh_key="${RUNNER_TEMP}/hivy-kubernetes-tunnel"
known_hosts="${RUNNER_TEMP}/hivy-kubernetes-known-hosts"
control_path="/tmp/hivy-k8s-${GITHUB_RUN_ID:-$$}-${GITHUB_RUN_ATTEMPT:-0}-%C"
local_port="${K8S_TUNNEL_LOCAL_PORT:-16443}"

umask 077
printf '%s' "${KUBE_CONFIG_B64}" | base64 --decode >"${KUBECONFIG}"
printf '%s' "${K8S_TUNNEL_SSH_KEY_B64}" | base64 --decode >"${ssh_key}"
printf '%s' "${K8S_TUNNEL_KNOWN_HOSTS_B64}" | base64 --decode >"${known_hosts}"

connected=false
for host in ${K8S_TUNNEL_HOSTS}; do
  if ssh -fNT \
    -M \
    -S "${control_path}" \
    -i "${ssh_key}" \
    -o BatchMode=yes \
    -o ConnectTimeout=10 \
    -o ExitOnForwardFailure=yes \
    -o IdentitiesOnly=yes \
    -o ServerAliveInterval=15 \
    -o ServerAliveCountMax=3 \
    -o StrictHostKeyChecking=yes \
    -o UserKnownHostsFile="${known_hosts}" \
    -L "127.0.0.1:${local_port}:127.0.0.1:6443" \
    "${K8S_TUNNEL_USER}@${host}"; then
    for _ in {1..10}; do
      if kubectl get --raw=/readyz >/dev/null 2>&1; then
        connected=true
        break 2
      fi
      sleep 1
    done
    ssh -S "${control_path}" -O exit "${K8S_TUNNEL_USER}@${host}" >/dev/null 2>&1 || true
  fi
done

if [[ "${connected}" != "true" ]]; then
  echo "failed to establish the restricted Kubernetes API tunnel" >&2
  exit 1
fi
