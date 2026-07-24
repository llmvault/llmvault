#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

mkdir -p "${tmp}/bin"
ssh_log="${tmp}/ssh.log"
github_env="${tmp}/github.env"

cat >"${tmp}/bin/ssh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"${FAKE_SSH_LOG}"
exit 0
EOF
chmod +x "${tmp}/bin/ssh"

cat >"${tmp}/bin/kubectl" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "${tmp}/bin/kubectl"

encoded_kubeconfig="$(printf 'apiVersion: v1\n' | base64)"
encoded_key="$(printf 'test-key\n' | base64)"
encoded_hosts="$(printf 'example.test ssh-ed25519 test\n' | base64)"

PATH="${tmp}/bin:${PATH}" \
FAKE_SSH_LOG="${ssh_log}" \
KUBE_CONFIG_B64="${encoded_kubeconfig}" \
K8S_TUNNEL_SSH_KEY_B64="${encoded_key}" \
K8S_TUNNEL_KNOWN_HOSTS_B64="${encoded_hosts}" \
K8S_TUNNEL_HOSTS="example.test" \
K8S_TUNNEL_USER="hivy-deploy-production" \
HIVY_MICROSANDBOX_RUNNER_FORWARD_TARGETS="18081:10.80.1.2:8081 18082:10.80.1.3:8081" \
RUNNER_TEMP="${tmp}" \
GITHUB_ENV="${github_env}" \
bash "${repo_root}/scripts/deploy/setup-kubernetes-tunnel.sh"

grep -Fq -- '-L 127.0.0.1:16443:127.0.0.1:6443' "${ssh_log}"
grep -Fq -- '-L 127.0.0.1:18081:10.80.1.2:8081' "${ssh_log}"
grep -Fq -- '-L 127.0.0.1:18082:10.80.1.3:8081' "${ssh_log}"
grep -Fq 'HIVY_MICROSANDBOX_RUNNER_URLS=http://127.0.0.1:18081 http://127.0.0.1:18082 ' "${github_env}"

: >"${ssh_log}"
PATH="${tmp}/bin:${PATH}" \
FAKE_SSH_LOG="${ssh_log}" \
KUBE_CONFIG_B64="${encoded_kubeconfig}" \
K8S_TUNNEL_SSH_KEY_B64="${encoded_key}" \
K8S_TUNNEL_KNOWN_HOSTS_B64="${encoded_hosts}" \
K8S_TUNNEL_HOSTS="example.test" \
K8S_TUNNEL_USER="hivy-deploy-staging" \
RUNNER_TEMP="${tmp}" \
bash "${repo_root}/scripts/deploy/setup-kubernetes-tunnel.sh"

grep -Fq -- '-L 127.0.0.1:16443:127.0.0.1:6443' "${ssh_log}"
if grep -Fq -- '10.80.1.2:8081' "${ssh_log}"; then
  echo "staging tunnel unexpectedly received a production runner forward" >&2
  exit 1
fi

if PATH="${tmp}/bin:${PATH}" \
  FAKE_SSH_LOG="${ssh_log}" \
  KUBE_CONFIG_B64="${encoded_kubeconfig}" \
  K8S_TUNNEL_SSH_KEY_B64="${encoded_key}" \
  K8S_TUNNEL_KNOWN_HOSTS_B64="${encoded_hosts}" \
  K8S_TUNNEL_HOSTS="example.test" \
  K8S_TUNNEL_USER="hivy-deploy-production" \
  HIVY_MICROSANDBOX_RUNNER_FORWARD_TARGETS='18081:10.80.1.2:8081;touch-bad' \
  RUNNER_TEMP="${tmp}" \
  GITHUB_ENV="${github_env}" \
  bash "${repo_root}/scripts/deploy/setup-kubernetes-tunnel.sh" >/dev/null 2>&1; then
  echo "invalid runner forward target was accepted" >&2
  exit 1
fi

echo "setup-kubernetes-tunnel tests passed"
