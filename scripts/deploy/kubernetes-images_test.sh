#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
deploy_script="${script_dir}/kubernetes-images.sh"
test_root="$(mktemp -d)"
trap 'rm -rf "${test_root}"' EXIT

fake_bin="${test_root}/bin"
mkdir -p "${fake_bin}"

cat >"${fake_bin}/kubectl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

printf '%s\n' "$*" >>"${FAKE_KUBECTL_LOG}"

deployment=""
previous=""
for argument in "$@"; do
  if [[ "${previous}" == "deployment" ]]; then
    deployment="${argument}"
    break
  fi
  previous="${argument}"
done

if [[ " $* " == *" get deployment "* ]]; then
  count_file="${FAKE_KUBECTL_STATE_DIR}/${deployment}.count"
  count=0
  if [[ -f "${count_file}" ]]; then
    count="$(cat "${count_file}")"
  fi
  count=$((count + 1))
  printf '%s' "${count}" >"${count_file}"

  container_name="${deployment}"
  image="${BACKEND_IMAGE}"
  if [[ "${deployment}" == "backend-api" ]]; then
    container_name="api"
  elif [[ "${deployment}" == "backend-worker" ]]; then
    container_name="worker"
  else
    container_name="web"
    image="${WEB_IMAGE}"
  fi

  if ((count == 1)); then
    jq -cn \
      --arg container_name "${container_name}" \
      --arg image "ghcr.io/usehivy/previous@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" \
      '{
        metadata: {generation: 1},
        spec: {
          replicas: 1,
          template: {
            metadata: {labels: {"app": "hivy"}},
            spec: {
              initContainers: [{name: "migrate", image: $image}],
              containers: [{name: $container_name, image: $image, env: []}]
            }
          }
        },
        status: {observedGeneration: 1, replicas: 1, updatedReplicas: 1, availableReplicas: 1, unavailableReplicas: 0}
      }'
    exit 0
  fi

  if [[ "${deployment}" == "web" ]]; then
    jq -cn \
      --arg image "${image}" \
      --arg revision "${DEPLOYMENT_REVISION}" \
      '{
        metadata: {generation: 2},
        spec: {
          replicas: 1,
          template: {
            metadata: {
              annotations: {
                "hivy.io/deployment-revision": $revision,
                "hivy.io/web-image": $image
              }
            },
            spec: {containers: [{name: "web", image: $image}]}
          }
        },
        status: {observedGeneration: 2, replicas: 1, updatedReplicas: 1, availableReplicas: 1, unavailableReplicas: 0}
      }'
    exit 0
  fi

  jq -cn \
    --arg container_name "${container_name}" \
    --arg image "${image}" \
    --arg runtime_tag "${SANDBOX_RUNTIME_IMAGE_TAG}" \
    --arg app_tag "${SANDBOX_APP_IMAGE_TAG}" \
    --arg revision "${DEPLOYMENT_REVISION}" \
    '{
      metadata: {generation: 2},
      spec: {
        replicas: 1,
        template: {
          metadata: {
            annotations: {
              "hivy.io/deployment-revision": $revision,
              "hivy.io/backend-image": $image
            }
          },
          spec: {
            initContainers: [{name: "migrate", image: $image}],
            containers: [{
              name: $container_name,
              image: $image,
              env: [
                {name: "HIVY_SANDBOXES_RUNTIME_IMAGE_TAG", value: $runtime_tag},
                {name: "HIVY_SANDBOXES_APP_IMAGE_TAG", value: $app_tag}
              ]
            }]
          }
        }
      },
      status: {observedGeneration: 2, replicas: 1, updatedReplicas: 1, availableReplicas: 1, unavailableReplicas: 0}
    }'
  exit 0
fi

if [[ "${FAKE_FAIL_WORKER_PATCH:-false}" == "true" ]] &&
  [[ " $* " == *" patch deployment backend-worker "* ]] &&
  [[ " $* " == *" --type=strategic "* ]] &&
  [[ ! -f "${FAKE_KUBECTL_STATE_DIR}/worker-patch-failed" ]]; then
  touch "${FAKE_KUBECTL_STATE_DIR}/worker-patch-failed"
  exit 42
fi

if [[ -n "${deployment}" ]]; then
  printf 'deployment.apps/%s\n' "${deployment}"
fi
EOF
chmod +x "${fake_bin}/kubectl"

export PATH="${fake_bin}:${PATH}"
export K8S_NAMESPACE="production"
export BACKEND_IMAGE="ghcr.io/usehivy/hivy@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
export WEB_IMAGE="ghcr.io/usehivy/web@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
export SANDBOX_RUNTIME_IMAGE_TAG="v8.3.0-amd64"
export SANDBOX_APP_IMAGE_TAG="v8.3.0-amd64"
export DEPLOYMENT_REVISION="v8.3.0"

happy_state="${test_root}/happy-state"
mkdir -p "${happy_state}"
export FAKE_KUBECTL_STATE_DIR="${happy_state}"
export FAKE_KUBECTL_LOG="${test_root}/happy.log"
unset FAKE_FAIL_WORKER_PATCH

bash "${deploy_script}" >"${test_root}/happy.out"
grep -Fq "Deployment tuple v8.3.0 is live in production" "${test_root}/happy.out"
grep -Fq '"name":"HIVY_SANDBOXES_RUNTIME_IMAGE_TAG","value":"v8.3.0-amd64"' "${FAKE_KUBECTL_LOG}"
grep -Fq '"name":"HIVY_SANDBOXES_APP_IMAGE_TAG","value":"v8.3.0-amd64"' "${FAKE_KUBECTL_LOG}"

pause_line="$(grep -nF "rollout pause deployment/backend-api deployment/backend-worker deployment/web" "${FAKE_KUBECTL_LOG}" | head -1 | cut -d: -f1)"
api_patch_line="$(grep -nF "patch deployment backend-api --type=strategic" "${FAKE_KUBECTL_LOG}" | head -1 | cut -d: -f1)"
worker_patch_line="$(grep -nF "patch deployment backend-worker --type=strategic" "${FAKE_KUBECTL_LOG}" | head -1 | cut -d: -f1)"
resume_line="$(grep -nF "rollout resume deployment/backend-api deployment/backend-worker deployment/web" "${FAKE_KUBECTL_LOG}" | head -1 | cut -d: -f1)"
if ! ((pause_line < api_patch_line && api_patch_line < worker_patch_line && worker_patch_line < resume_line)); then
  echo "backend deployments were not paused, patched, and resumed in order" >&2
  exit 1
fi

rollback_state="${test_root}/rollback-state"
mkdir -p "${rollback_state}"
export FAKE_KUBECTL_STATE_DIR="${rollback_state}"
export FAKE_KUBECTL_LOG="${test_root}/rollback.log"
export FAKE_FAIL_WORKER_PATCH=true

set +e
bash "${deploy_script}" >"${test_root}/rollback.out" 2>"${test_root}/rollback.err"
rollback_exit=$?
set -e
if ((rollback_exit == 0)); then
  echo "expected the simulated worker patch failure to fail the deployment" >&2
  exit 1
fi
if [[ "$(grep -cF -- "--type=json" "${FAKE_KUBECTL_LOG}")" -ne 3 ]]; then
  echo "expected all three previous pod templates to be restored" >&2
  exit 1
fi
grep -Fq "Previous deployment tuple restored" "${test_root}/rollback.err"
grep -Fq "rollout resume deployment/backend-api deployment/backend-worker deployment/web" "${FAKE_KUBECTL_LOG}"

echo "kubernetes-images deployment tests passed"
