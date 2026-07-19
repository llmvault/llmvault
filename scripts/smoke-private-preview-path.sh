#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
kubeconfig="${KUBECONFIG:-${repo_root}/kubernetes/config/kubeconfigs/k8s0/local.yaml}"
preview_host="${HIVY_SMOKE_PREVIEW_HOST:-preview.usehivy.com}"

export KUBECONFIG="${kubeconfig}"

bridge_ip="$(kubectl -n production get service microsandbox-preview-tls-bridge -o jsonpath='{.spec.clusterIP}')"
if [[ -z "${bridge_ip}" || "${bridge_ip}" == "None" ]]; then
  echo "preview TLS bridge has no ClusterIP" >&2
  exit 1
fi

for namespace in production staging; do
  pod="$(kubectl -n "${namespace}" get pods \
    -l app.kubernetes.io/name=hivy-backend,app.kubernetes.io/component=api \
    --field-selector=status.phase=Running \
    -o jsonpath='{.items[0].metadata.name}')"

  for dns_ip in $(kubectl -n kube-system get pods -l k8s-app=kube-dns \
    --field-selector=status.phase=Running -o jsonpath='{range .items[*]}{.status.podIP}{"\n"}{end}'); do
    resolved="$(kubectl -n "${namespace}" exec "${pod}" -- nslookup "${preview_host}" "${dns_ip}" | awk '/^Address: / { value = $2 } END { print value }')"
    if [[ "${resolved}" != "${bridge_ip}" ]]; then
      echo "${namespace}: DNS replica ${dns_ip} returned ${resolved}, expected ${bridge_ip}" >&2
      exit 1
    fi
  done

  resolved="$(kubectl -n "${namespace}" exec "${pod}" -- getent ahostsv4 "${preview_host}" | awk 'NR == 1 { print $1 }')"
  if [[ "${resolved}" != "${bridge_ip}" ]]; then
    echo "${namespace}: ${preview_host} resolved to ${resolved}, expected ${bridge_ip}" >&2
    exit 1
  fi

  probe="$(kubectl -n "${namespace}" exec "${pod}" -- \
    wget -S -O /dev/null --timeout=5 "https://${preview_host}/" 2>&1 || true)"
  if grep -Eq 'SSL_connect|Connection reset|bad address' <<<"${probe}"; then
    echo "${namespace}: TLS did not reach the preview Gateway" >&2
    echo "${probe}" >&2
    exit 1
  fi
  if ! grep -Eq 'HTTP/[0-9.]+' <<<"${probe}"; then
    echo "${namespace}: preview probe produced no HTTP response" >&2
    echo "${probe}" >&2
    exit 1
  fi

  echo "${namespace}: ${preview_host} resolved privately and completed TLS"
done
