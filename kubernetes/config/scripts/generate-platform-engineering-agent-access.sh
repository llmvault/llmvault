#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
config_root="$(cd "${script_dir}/.." && pwd)"
access_config="${config_root}/env/platform-engineering/platform-engineering-agent.config.env"
access_env="${config_root}/env/platform-engineering/platform-engineering-agent.env"
access_kubeconfig="${config_root}/kubeconfigs/platform-engineering-agent.yaml"
ssh_key="${config_root}/credentials/platform-engineering-agent/tunnel"
known_hosts="${config_root}/credentials/platform-engineering-agent/known_hosts"
admin_kubeconfig="${K8S_ADMIN_KUBECONFIG:-${config_root}/kubeconfigs/k8s0/local.yaml}"

if [[ ! -f "${access_config}" ]]; then
  echo "missing access configuration: ${access_config}" >&2
  exit 1
fi

# This file contains committed, non-secret deployment configuration only.
# shellcheck disable=SC1090
source "${access_config}"

for required_file in "${admin_kubeconfig}" "${ssh_key}" "${known_hosts}"; do
  if [[ ! -f "${required_file}" ]]; then
    echo "missing Platform Engineering Agent input: ${required_file}" >&2
    exit 1
  fi
done

umask 077
mkdir -p "$(dirname "${access_env}")" "$(dirname "${access_kubeconfig}")"

token_b64=""
for _ in {1..30}; do
  token_b64="$(
    kubectl --kubeconfig "${admin_kubeconfig}" \
      --namespace platform-engineering \
      get secret platform-engineering-agent-token \
      --output=jsonpath='{.data.token}'
  )"
  if [[ -n "${token_b64}" ]]; then
    break
  fi
  sleep 1
done
if [[ -z "${token_b64}" ]]; then
  echo "the Platform Engineering Agent token has not been populated" >&2
  exit 1
fi
token="$(printf '%s' "${token_b64}" | base64 --decode)"

ca_data="$(
  kubectl --kubeconfig "${admin_kubeconfig}" \
    config view --raw --minify \
    --output=jsonpath='{.clusters[0].cluster.certificate-authority-data}'
)"
if [[ -z "${ca_data}" ]]; then
  echo "the administrator kubeconfig does not embed certificate authority data" >&2
  exit 1
fi

temporary_kubeconfig="${access_kubeconfig}.tmp"
rm -f "${temporary_kubeconfig}"
kubectl --kubeconfig "${temporary_kubeconfig}" config set-cluster hivy \
  --server="https://127.0.0.1:${K8S_TUNNEL_LOCAL_PORT}" >/dev/null
kubectl --kubeconfig "${temporary_kubeconfig}" config set \
  clusters.hivy.certificate-authority-data "${ca_data}" >/dev/null
kubectl --kubeconfig "${temporary_kubeconfig}" config set-credentials \
  platform-engineering-agent --token="${token}" >/dev/null
kubectl --kubeconfig "${temporary_kubeconfig}" config set-context \
  platform-engineering-agent \
  --cluster=hivy \
  --user=platform-engineering-agent \
  --namespace=production >/dev/null
kubectl --kubeconfig "${temporary_kubeconfig}" config use-context \
  platform-engineering-agent >/dev/null
mv "${temporary_kubeconfig}" "${access_kubeconfig}"
chmod 0600 "${access_kubeconfig}"

encode_file() {
  base64 <"$1" | tr -d '\r\n'
}

{
  printf 'KUBE_CONFIG_B64=%s\n' "$(encode_file "${access_kubeconfig}")"
  printf 'K8S_TUNNEL_SSH_KEY_B64=%s\n' "$(encode_file "${ssh_key}")"
  printf 'K8S_TUNNEL_KNOWN_HOSTS_B64=%s\n' "$(encode_file "${known_hosts}")"
  printf 'K8S_TUNNEL_HOSTS="%s"\n' "${K8S_TUNNEL_HOSTS}"
  printf 'K8S_TUNNEL_USER=%s\n' "${K8S_TUNNEL_USER}"
  printf 'K8S_TUNNEL_LOCAL_PORT=%s\n' "${K8S_TUNNEL_LOCAL_PORT}"
  printf 'K8S_TUNNEL_STATE_DIR=%s\n' "${K8S_TUNNEL_STATE_DIR}"
  printf 'KUBECONFIG=%s\n' "${KUBECONFIG}"
  printf 'KUBECTL_VERSION=%s\n' "${KUBECTL_VERSION}"
} >"${access_env}"
chmod 0600 "${access_env}"

unset token token_b64
echo "generated Git-ignored Platform Engineering Agent access bundle"
echo "  kubernetes/config/env/platform-engineering/platform-engineering-agent.env"
echo "  kubernetes/config/kubeconfigs/platform-engineering-agent.yaml"
