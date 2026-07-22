#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
setup_script="${script_dir}/setup-kubernetes.sh"
terminate_script="${script_dir}/terminate-kubernetes.sh"
temporary_root="$(mktemp -d "${TMPDIR:-/tmp}/hivy-kubernetes-access-test.XXXXXX")"
trap 'rm -rf "${temporary_root}"' EXIT

failures=0

assert_file() {
  local path="$1"
  local name="$2"
  if [[ ! -f "${path}" ]]; then
    echo "FAIL [${name}]: missing file ${path}"
    failures=$(( failures + 1 ))
  else
    echo "ok   [${name}]"
  fi
}

assert_absent() {
  local path="$1"
  local name="$2"
  if [[ -e "${path}" ]]; then
    echo "FAIL [${name}]: unexpected path ${path}"
    failures=$(( failures + 1 ))
  else
    echo "ok   [${name}]"
  fi
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  local name="$3"
  case "${haystack}" in
    *"${needle}"*) echo "ok   [${name}]" ;;
    *)
      echo "FAIL [${name}]: output missing '${needle}'"
      echo "${haystack}"
      failures=$(( failures + 1 ))
      ;;
  esac
}

make_fake_tools() {
  local fake_bin="$1"
  mkdir -p "${fake_bin}"

  cat >"${fake_bin}/kubectl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$*" == *"get --raw=/readyz"* ]]; then
  echo ok
  exit 0
fi
echo "unexpected kubectl invocation: $*" >&2
exit 1
EOF

  cat >"${fake_bin}/ssh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
socket=""
operation=""
previous=""
for argument in "$@"; do
  if [[ "${previous}" == "-S" ]]; then
    socket="${argument}"
  fi
  if [[ "${previous}" == "-O" ]]; then
    operation="${argument}"
  fi
  previous="${argument}"
done

case "${operation}" in
  check)
    [[ -e "${socket}" ]]
    ;;
  exit)
    rm -f "${socket}"
    ;;
  *)
    : >"${socket}"
    ;;
esac
EOF
  chmod 0755 "${fake_bin}/kubectl" "${fake_bin}/ssh"
}

run_happy_path() {
  local case_root="${temporary_root}/happy"
  local fake_bin="${case_root}/bin"
  local state_dir="${case_root}/state"
  local output
  make_fake_tools "${fake_bin}"

  output="$(
    env \
      PATH="${fake_bin}:/usr/bin:/bin" \
      HOME="${case_root}/home" \
      KUBE_CONFIG_B64="$(printf kubeconfig | base64)" \
      K8S_TUNNEL_SSH_KEY_B64="$(printf private-key | base64)" \
      K8S_TUNNEL_KNOWN_HOSTS_B64="$(printf known-hosts | base64)" \
      K8S_TUNNEL_HOSTS="k8s0.example.test k8s1.example.test" \
      K8S_TUNNEL_USER="hivy-deploy-platform-engineering-agent" \
      K8S_TUNNEL_STATE_DIR="${state_dir}" \
      KUBECONFIG="${state_dir}/kubeconfig" \
      "${setup_script}"
  )"

  assert_contains "${output}" \
    "can now successfully run kubectl commands" \
    "setup reports readiness"
  assert_file "${state_dir}/kubeconfig" "setup writes kubeconfig"
  assert_file "${state_dir}/tunnel-key" "setup writes SSH key"
  assert_file "${state_dir}/connected-host" "setup records selected host"

  output="$(
    env \
      PATH="${fake_bin}:/usr/bin:/bin" \
      HOME="${case_root}/home" \
      K8S_TUNNEL_STATE_DIR="${state_dir}" \
      "${terminate_script}"
  )"
  assert_contains "${output}" \
    "tunnel stopped and local session credentials removed" \
    "termination reports success"
  assert_absent "${state_dir}/kubeconfig" "termination removes kubeconfig"
  assert_absent "${state_dir}/tunnel-key" "termination removes SSH key"
  assert_absent "${state_dir}/ssh-control" "termination removes control socket"
}

run_missing_environment_test() {
  local case_root="${temporary_root}/missing"
  local fake_bin="${case_root}/bin"
  local output rc
  make_fake_tools "${fake_bin}"

  set +e
  output="$(
    env -i \
      PATH="${fake_bin}:/usr/bin:/bin" \
      HOME="${case_root}/home" \
      "${setup_script}" 2>&1
  )"
  rc=$?
  set -e

  if [[ "${rc}" -eq 0 ]]; then
    echo "FAIL [missing environment fails]: setup unexpectedly succeeded"
    failures=$(( failures + 1 ))
  else
    echo "ok   [missing environment fails]"
  fi
  assert_contains "${output}" "KUBE_CONFIG_B64 is required" \
    "missing environment names required variable"
}

run_kubectl_install_test() {
  local case_root="${temporary_root}/install"
  local fake_bin="${case_root}/bin"
  local install_bin="${case_root}/install-bin"
  local fixture="${case_root}/kubectl-fixture"
  local state_dir="${case_root}/state"
  local output
  mkdir -p "${fake_bin}" "${install_bin}"

  cat >"${fixture}" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$*" == *"get --raw=/readyz"* ]]; then
  echo ok
  exit 0
fi
exit 0
EOF
  chmod 0755 "${fixture}"

  cat >"${fake_bin}/curl" <<EOF
#!/usr/bin/env bash
set -euo pipefail
destination=""
url=""
previous=""
for argument in "\$@"; do
  if [[ "\${previous}" == "--output" ]]; then
    destination="\${argument}"
  fi
  previous="\${argument}"
  url="\${argument}"
done
if [[ "\${url}" == *.sha256 ]]; then
  shasum -a 256 "${fixture}" | awk '{print \$1}' >"\${destination}"
else
  cp "${fixture}" "\${destination}"
fi
EOF

  cat >"${fake_bin}/ssh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
socket=""
operation=""
previous=""
for argument in "$@"; do
  [[ "${previous}" == "-S" ]] && socket="${argument}"
  [[ "${previous}" == "-O" ]] && operation="${argument}"
  previous="${argument}"
done
if [[ "${operation}" == "exit" ]]; then
  rm -f "${socket}"
else
  : >"${socket}"
fi
EOF
  chmod 0755 "${fake_bin}/curl" "${fake_bin}/ssh"

  output="$(
    env \
      PATH="${fake_bin}:${install_bin}:/usr/bin:/bin" \
      HOME="${case_root}/home" \
      KUBE_CONFIG_B64="$(printf kubeconfig | base64)" \
      K8S_TUNNEL_SSH_KEY_B64="$(printf private-key | base64)" \
      K8S_TUNNEL_KNOWN_HOSTS_B64="$(printf known-hosts | base64)" \
      K8S_TUNNEL_HOSTS="k8s0.example.test" \
      K8S_TUNNEL_USER="hivy-deploy-platform-engineering-agent" \
      K8S_TUNNEL_STATE_DIR="${state_dir}" \
      KUBECONFIG="${state_dir}/kubeconfig" \
      KUBECTL_INSTALL_DIR="${install_bin}" \
      KUBECTL_VERSION="v1.35.6" \
      "${setup_script}"
  )"

  assert_contains "${output}" "kubectl is not installed" \
    "setup installs missing kubectl"
  assert_file "${install_bin}/kubectl" "verified kubectl is installed"

  env \
    PATH="${fake_bin}:${install_bin}:/usr/bin:/bin" \
    HOME="${case_root}/home" \
    K8S_TUNNEL_STATE_DIR="${state_dir}" \
    "${terminate_script}" >/dev/null
}

run_happy_path
run_missing_environment_test
run_kubectl_install_test

if (( failures > 0 )); then
  echo
  echo "${failures} check(s) FAILED"
  exit 1
fi

echo
echo "All Platform Engineering Agent access checks passed."
