#!/usr/bin/env bash
set -euo pipefail

required_variables=(
  KUBE_CONFIG_B64
  K8S_TUNNEL_SSH_KEY_B64
  K8S_TUNNEL_KNOWN_HOSTS_B64
  K8S_TUNNEL_HOSTS
  K8S_TUNNEL_USER
)

for variable_name in "${required_variables[@]}"; do
  if [[ -z "${!variable_name:-}" ]]; then
    echo "${variable_name} is required" >&2
    exit 1
  fi
done

if ! command -v ssh >/dev/null 2>&1; then
  echo "ssh is required to open the Kubernetes API tunnel" >&2
  exit 1
fi

state_dir_input="${K8S_TUNNEL_STATE_DIR:-${TMPDIR:-/tmp}/hivy-platform-engineering-agent}"
case "${state_dir_input}" in
  ""|/|/tmp|"${HOME:-__unset_home__}")
    echo "refusing unsafe K8S_TUNNEL_STATE_DIR: ${state_dir_input}" >&2
    exit 1
    ;;
esac

umask 077
mkdir -p "${state_dir_input}"
chmod 0700 "${state_dir_input}"
state_dir="$(cd "${state_dir_input}" && pwd -P)"
kubeconfig_input="${KUBECONFIG:-${state_dir}/kubeconfig}"
mkdir -p "$(dirname "${kubeconfig_input}")"
kubeconfig="$(cd "$(dirname "${kubeconfig_input}")" && pwd -P)/$(basename "${kubeconfig_input}")"

case "${kubeconfig}" in
  "${state_dir}"/*) ;;
  *)
    echo "KUBECONFIG must be stored inside K8S_TUNNEL_STATE_DIR" >&2
    exit 1
    ;;
esac

ssh_key="${state_dir}/tunnel-key"
known_hosts="${state_dir}/known-hosts"
control_socket="${state_dir}/ssh-control"
connected_host_file="${state_dir}/connected-host"
connected_user_file="${state_dir}/connected-user"

download_file() {
  local url="$1"
  local destination="$2"

  if command -v curl >/dev/null 2>&1; then
    curl --fail --location --silent --show-error \
      --output "${destination}" "${url}"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget --quiet --output-document="${destination}" "${url}"
    return
  fi

  echo "curl or wget is required to install kubectl" >&2
  exit 1
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
    return
  fi

  echo "sha256sum or shasum is required to verify kubectl" >&2
  exit 1
}

install_kubectl() {
  local version="${KUBECTL_VERSION:-v1.35.6}"
  local os arch install_dir temporary_dir binary checksum expected actual

  if [[ ! "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "invalid KUBECTL_VERSION: ${version}" >&2
    exit 1
  fi

  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "${os}" in
    linux|darwin) ;;
    *)
      echo "automatic kubectl installation does not support ${os}" >&2
      exit 1
      ;;
  esac

  arch="$(uname -m)"
  case "${arch}" in
    x86_64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *)
      echo "automatic kubectl installation does not support ${arch}" >&2
      exit 1
      ;;
  esac

  if [[ -n "${KUBECTL_INSTALL_DIR:-}" ]]; then
    install_dir="${KUBECTL_INSTALL_DIR}"
  elif [[ -w /usr/local/bin ]]; then
    install_dir=/usr/local/bin
  elif command -v sudo >/dev/null 2>&1 && sudo -n true >/dev/null 2>&1; then
    install_dir=/usr/local/bin
  elif [[ -n "${HOME:-}" && ":${PATH}:" == *":${HOME}/.local/bin:"* ]]; then
    install_dir="${HOME}/.local/bin"
  else
    echo "kubectl is missing and no writable installation directory is on PATH" >&2
    echo "install kubectl or add a writable directory to PATH with KUBECTL_INSTALL_DIR" >&2
    exit 1
  fi

  if [[ ! -d "${install_dir}" ]]; then
    if [[ -w "$(dirname "${install_dir}")" ]]; then
      mkdir -p "${install_dir}"
    else
      sudo mkdir -p "${install_dir}"
    fi
  fi
  temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/hivy-kubectl.XXXXXX")"
  binary="${temporary_dir}/kubectl"
  checksum="${temporary_dir}/kubectl.sha256"

  download_file \
    "https://dl.k8s.io/release/${version}/bin/${os}/${arch}/kubectl" \
    "${binary}"
  download_file \
    "https://dl.k8s.io/release/${version}/bin/${os}/${arch}/kubectl.sha256" \
    "${checksum}"

  expected="$(tr -d '[:space:]' <"${checksum}")"
  actual="$(sha256_file "${binary}")"
  if [[ -z "${expected}" || "${expected}" != "${actual}" ]]; then
    echo "downloaded kubectl checksum verification failed" >&2
    exit 1
  fi

  chmod 0755 "${binary}"
  if [[ -w "${install_dir}" ]]; then
    install -m 0755 "${binary}" "${install_dir}/kubectl"
  else
    sudo install -m 0755 "${binary}" "${install_dir}/kubectl"
  fi

  if [[ ":${PATH}:" != *":${install_dir}:"* ]]; then
    echo "kubectl was installed to ${install_dir}, but that directory is not on PATH" >&2
    exit 1
  fi
  hash -r
  rm -f "${binary}" "${checksum}"
  rmdir "${temporary_dir}" 2>/dev/null || true
}

if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl is not installed; installing a verified ${KUBECTL_VERSION:-v1.35.6} binary"
  install_kubectl
fi

decode_secret() {
  local encoded="$1"
  local destination="$2"
  local temporary="${destination}.tmp"

  printf '%s' "${encoded}" | base64 --decode >"${temporary}"
  if [[ ! -s "${temporary}" ]]; then
    rm -f "${temporary}"
    echo "decoded credential is empty: ${destination}" >&2
    exit 1
  fi
  chmod 0600 "${temporary}"
  mv "${temporary}" "${destination}"
}

decode_secret "${KUBE_CONFIG_B64}" "${kubeconfig}"
decode_secret "${K8S_TUNNEL_SSH_KEY_B64}" "${ssh_key}"
decode_secret "${K8S_TUNNEL_KNOWN_HOSTS_B64}" "${known_hosts}"
export KUBECONFIG="${kubeconfig}"

read -r -a tunnel_hosts <<<"${K8S_TUNNEL_HOSTS}"
if (( ${#tunnel_hosts[@]} == 0 )); then
  echo "K8S_TUNNEL_HOSTS must contain at least one host" >&2
  exit 1
fi

local_port="${K8S_TUNNEL_LOCAL_PORT:-16443}"
if [[ ! "${local_port}" =~ ^[0-9]+$ ]] || (( local_port < 1024 || local_port > 65535 )); then
  echo "K8S_TUNNEL_LOCAL_PORT must be between 1024 and 65535" >&2
  exit 1
fi

ssh_options=(
  -S "${control_socket}"
  -i "${ssh_key}"
  -o BatchMode=yes
  -o ConnectTimeout=10
  -o ExitOnForwardFailure=yes
  -o IdentitiesOnly=yes
  -o ServerAliveInterval=15
  -o ServerAliveCountMax=3
  -o StrictHostKeyChecking=yes
  -o "UserKnownHostsFile=${known_hosts}"
)

if [[ -f "${connected_host_file}" && -f "${connected_user_file}" ]]; then
  connected_host="$(<"${connected_host_file}")"
  connected_user="$(<"${connected_user_file}")"
  if ssh "${ssh_options[@]}" -O check "${connected_user}@${connected_host}" >/dev/null 2>&1 && \
    kubectl --request-timeout=15s get --raw=/readyz >/dev/null 2>&1; then
    echo "Kubernetes access is already ready."
    echo "The Platform Engineering Agent can now successfully run kubectl commands."
    exit 0
  fi
fi

rm -f "${control_socket}" "${connected_host_file}" "${connected_user_file}"

for host in "${tunnel_hosts[@]}"; do
  if ssh -fNT -M "${ssh_options[@]}" \
    -L "127.0.0.1:${local_port}:127.0.0.1:6443" \
    "${K8S_TUNNEL_USER}@${host}"; then
    ready=false
    for _ in {1..10}; do
      if kubectl --request-timeout=15s get --raw=/readyz >/dev/null 2>&1; then
        ready=true
        break
      fi
      sleep 1
    done

    if [[ "${ready}" == "true" ]]; then
      printf '%s\n' "${host}" >"${connected_host_file}"
      printf '%s\n' "${K8S_TUNNEL_USER}" >"${connected_user_file}"
      chmod 0600 "${connected_host_file}" "${connected_user_file}"
      echo "Kubernetes access is ready through ${host} on local port ${local_port}."
      echo "The Platform Engineering Agent can now successfully run kubectl commands."
      exit 0
    fi

    ssh "${ssh_options[@]}" -O exit \
      "${K8S_TUNNEL_USER}@${host}" >/dev/null 2>&1 || true
    rm -f "${control_socket}"
  fi
done

echo "failed to establish the restricted Kubernetes API tunnel" >&2
exit 1
