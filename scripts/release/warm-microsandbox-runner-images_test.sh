#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

mkdir -p "${tmp}/bin"
curl_log="${tmp}/curl.log"
manifest="${tmp}/release-manifest.json"

cat >"${tmp}/bin/curl" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"${FAKE_CURL_LOG}"
printf '{"status":"ok"}\n'
EOF
chmod +x "${tmp}/bin/curl"

bash "${repo_root}/scripts/release/write-manifest.sh" \
  v8.4.1 "${manifest}" 0123456789abcdef0123456789abcdef01234567

PATH="${tmp}/bin:${PATH}" \
FAKE_CURL_LOG="${curl_log}" \
HIVY_MICROSANDBOX_RUNNER_API_TOKEN="test-runner-token" \
HIVY_SANDBOXES_RUNTIME_IMAGE_ARCH_SUFFIX=amd64 \
HIVY_SANDBOXES_APP_IMAGE_ARCH_SUFFIX=amd64 \
bash "${repo_root}/scripts/release/warm-microsandbox-runner-images.sh" \
  "${manifest}" \
  http://127.0.0.1:18081 \
  http://127.0.0.1:18082

create_count="$(grep -Ec -- '-X POST http://127\.0\.0\.1:[0-9]+/v1/sandboxes -H' "${curl_log}" || true)"
if [[ "${create_count}" != "6" ]]; then
  echo "create calls = ${create_count}, want 6" >&2
  cat "${curl_log}" >&2
  exit 1
fi

for image in \
  ghcr.io/usehivy/hivy-sandboxes-runtime:v8.4.1-amd64 \
  ghcr.io/usehivy/hivy-sandboxes-runtime-developers:v8.4.1-amd64 \
  ghcr.io/usehivy/hivy-app:v8.4.1-amd64; do
  count="$(grep -c -- "${image}" "${curl_log}" || true)"
  if [[ "${count}" != "2" ]]; then
    echo "${image} warm calls = ${count}, want 2" >&2
    exit 1
  fi
done

if PATH="${tmp}/bin:${PATH}" \
  FAKE_CURL_LOG="${curl_log}" \
  HIVY_MICROSANDBOX_RUNNER_API_TOKEN="test-runner-token" \
  bash "${repo_root}/scripts/release/warm-microsandbox-runner-images.sh" \
    "${manifest}" >/dev/null 2>&1; then
  echo "warm-up succeeded without runner URLs" >&2
  exit 1
fi

echo "warm-microsandbox-runner-images tests passed"
