#!/usr/bin/env bash
set -euo pipefail

: "${K8S_NAMESPACE:?K8S_NAMESPACE is required}"
: "${BACKEND_IMAGE:?BACKEND_IMAGE is required}"
: "${WEB_IMAGE:?WEB_IMAGE is required}"
: "${SANDBOX_RUNTIME_IMAGE_TAG:?SANDBOX_RUNTIME_IMAGE_TAG is required}"
: "${SANDBOX_APP_IMAGE_TAG:?SANDBOX_APP_IMAGE_TAG is required}"
: "${DEPLOYMENT_REVISION:?DEPLOYMENT_REVISION is required}"

if [[ ! "${BACKEND_IMAGE}" =~ ^ghcr\.io/usehivy/hivy@sha256:[a-f0-9]{64}$ ]]; then
  echo "BACKEND_IMAGE must be an immutable usehivy/hivy digest" >&2
  exit 1
fi
if [[ ! "${WEB_IMAGE}" =~ ^ghcr\.io/usehivy/web@sha256:[a-f0-9]{64}$ ]]; then
  echo "WEB_IMAGE must be an immutable usehivy/web digest" >&2
  exit 1
fi
if [[ ! "${SANDBOX_RUNTIME_IMAGE_TAG}" =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$ ]]; then
  echo "SANDBOX_RUNTIME_IMAGE_TAG must be a valid container image tag" >&2
  exit 1
fi
if [[ ! "${SANDBOX_APP_IMAGE_TAG}" =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$ ]]; then
  echo "SANDBOX_APP_IMAGE_TAG must be a valid container image tag" >&2
  exit 1
fi
if [[ ! "${DEPLOYMENT_REVISION}" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$ ]]; then
  echo "DEPLOYMENT_REVISION must contain only tag-safe characters" >&2
  exit 1
fi

deployment_state() {
  kubectl --namespace "${K8S_NAMESPACE}" get deployment "$1" --output=json
}

api_before="$(deployment_state backend-api)"
worker_before="$(deployment_state backend-worker)"
web_before="$(deployment_state web)"
api_template_before="$(jq -c '.spec.template' <<<"${api_before}")"
worker_template_before="$(jq -c '.spec.template' <<<"${worker_before}")"
web_template_before="$(jq -c '.spec.template' <<<"${web_before}")"

deployment_complete=false

wait_for_rollout() {
  local deployment="$1"
  local deadline=$((SECONDS + 600))

  while ((SECONDS < deadline)); do
    local state generation observed desired total updated available unavailable
    state="$(deployment_state "${deployment}")"
    generation="$(jq -r '.metadata.generation' <<<"${state}")"
    observed="$(jq -r '.status.observedGeneration // 0' <<<"${state}")"
    desired="$(jq -r '.spec.replicas // 1' <<<"${state}")"
    total="$(jq -r '.status.replicas // 0' <<<"${state}")"
    updated="$(jq -r '.status.updatedReplicas // 0' <<<"${state}")"
    available="$(jq -r '.status.availableReplicas // 0' <<<"${state}")"
    unavailable="$(jq -r '.status.unavailableReplicas // 0' <<<"${state}")"

    if ((observed >= generation && total == desired && updated == desired && available == desired && unavailable == 0)); then
      echo "${deployment} rollout complete"
      return 0
    fi
    sleep 5
  done

  echo "${deployment} rollout did not complete within 10 minutes" >&2
  return 1
}

restore_template() {
  local deployment="$1"
  local template="$2"
  local patch
  patch="$(jq -cn --argjson template "${template}" \
    '[{"op":"replace","path":"/spec/template","value":$template}]')"
  kubectl --namespace "${K8S_NAMESPACE}" patch deployment "${deployment}" \
    --type=json --patch "${patch}" --output=name
}

rollback_on_failure() {
  local status="$1"
  if [[ "${deployment_complete}" == "true" ]]; then
    return "${status}"
  fi

  trap - EXIT
  set +e
  echo "Deployment failed; restoring the previous pod templates" >&2
  kubectl --namespace "${K8S_NAMESPACE}" rollout pause \
    deployment/backend-api deployment/backend-worker deployment/web >/dev/null 2>&1
  restore_template backend-api "${api_template_before}"
  api_restore_status=$?
  restore_template backend-worker "${worker_template_before}"
  worker_restore_status=$?
  restore_template web "${web_template_before}"
  web_restore_status=$?
  kubectl --namespace "${K8S_NAMESPACE}" rollout resume \
    deployment/backend-api deployment/backend-worker deployment/web >/dev/null 2>&1

  rollback_status=0
  if ((api_restore_status != 0 || worker_restore_status != 0 || web_restore_status != 0)); then
    echo "Automatic rollback could not restore every previous pod template" >&2
    rollback_status=1
  else
    wait_for_rollout backend-api || rollback_status=1
    wait_for_rollout backend-worker || rollback_status=1
    wait_for_rollout web || rollback_status=1
  fi
  if ((rollback_status != 0)); then
    echo "Automatic rollback did not complete; operator intervention is required" >&2
  else
    echo "Previous deployment tuple restored" >&2
  fi
  exit "${status}"
}
trap 'rollback_on_failure $?' EXIT

backend_patch() {
  local container_name="$1"
  jq -cn \
    --arg backend_image "${BACKEND_IMAGE}" \
    --arg container_name "${container_name}" \
    --arg runtime_tag "${SANDBOX_RUNTIME_IMAGE_TAG}" \
    --arg app_tag "${SANDBOX_APP_IMAGE_TAG}" \
    --arg revision "${DEPLOYMENT_REVISION}" \
    '{
      metadata: {
        annotations: {
          "kubernetes.io/change-cause": ("Deploy " + $revision)
        }
      },
      spec: {
        template: {
          metadata: {
            annotations: {
              "hivy.io/deployment-revision": $revision,
              "hivy.io/backend-image": $backend_image,
              "hivy.io/sandbox-runtime-image-tag": $runtime_tag,
              "hivy.io/sandbox-app-image-tag": $app_tag
            }
          },
          spec: {
            initContainers: [
              {
                name: "migrate",
                image: $backend_image
              }
            ],
            containers: [
              {
                name: $container_name,
                image: $backend_image,
                env: [
                  {
                    name: "HIVY_SANDBOXES_RUNTIME_IMAGE_TAG",
                    value: $runtime_tag
                  },
                  {
                    name: "HIVY_SANDBOXES_APP_IMAGE_TAG",
                    value: $app_tag
                  }
                ]
              }
            ]
          }
        }
      }
    }'
}

web_patch="$(jq -cn \
  --arg web_image "${WEB_IMAGE}" \
  --arg revision "${DEPLOYMENT_REVISION}" \
  '{
    metadata: {
      annotations: {
        "kubernetes.io/change-cause": ("Deploy " + $revision)
      }
    },
    spec: {
      template: {
        metadata: {
          annotations: {
            "hivy.io/deployment-revision": $revision,
            "hivy.io/web-image": $web_image
          }
        },
        spec: {
          containers: [
            {
              name: "web",
              image: $web_image
            }
          ]
        }
      }
    }
  }')"

kubectl --namespace "${K8S_NAMESPACE}" rollout pause \
  deployment/backend-api deployment/backend-worker deployment/web

kubectl --namespace "${K8S_NAMESPACE}" patch deployment backend-api \
  --type=strategic --patch "$(backend_patch api)" --output=name
kubectl --namespace "${K8S_NAMESPACE}" patch deployment backend-worker \
  --type=strategic --patch "$(backend_patch worker)" --output=name
kubectl --namespace "${K8S_NAMESPACE}" patch deployment web \
  --type=strategic --patch "${web_patch}" --output=name

kubectl --namespace "${K8S_NAMESPACE}" rollout resume \
  deployment/backend-api deployment/backend-worker deployment/web

wait_for_rollout backend-api
wait_for_rollout backend-worker
wait_for_rollout web

verify_backend_tuple() {
  local deployment="$1"
  local container_name="$2"
  local state
  state="$(deployment_state "${deployment}")"
  jq -e \
    --arg backend_image "${BACKEND_IMAGE}" \
    --arg container_name "${container_name}" \
    --arg runtime_tag "${SANDBOX_RUNTIME_IMAGE_TAG}" \
    --arg app_tag "${SANDBOX_APP_IMAGE_TAG}" \
    --arg revision "${DEPLOYMENT_REVISION}" \
    '
      .spec.template.metadata.annotations["hivy.io/deployment-revision"] == $revision
      and .spec.template.metadata.annotations["hivy.io/backend-image"] == $backend_image
      and any(.spec.template.spec.initContainers[]; .name == "migrate" and .image == $backend_image)
      and any(.spec.template.spec.containers[]; .name == $container_name and .image == $backend_image)
      and any(
        .spec.template.spec.containers[] | select(.name == $container_name) | .env[];
        .name == "HIVY_SANDBOXES_RUNTIME_IMAGE_TAG" and .value == $runtime_tag
      )
      and any(
        .spec.template.spec.containers[] | select(.name == $container_name) | .env[];
        .name == "HIVY_SANDBOXES_APP_IMAGE_TAG" and .value == $app_tag
      )
    ' <<<"${state}" >/dev/null
}

verify_backend_tuple backend-api api
verify_backend_tuple backend-worker worker

web_state="$(deployment_state web)"
jq -e \
  --arg web_image "${WEB_IMAGE}" \
  --arg revision "${DEPLOYMENT_REVISION}" \
  '
    .spec.template.metadata.annotations["hivy.io/deployment-revision"] == $revision
    and .spec.template.metadata.annotations["hivy.io/web-image"] == $web_image
    and any(.spec.template.spec.containers[]; .name == "web" and .image == $web_image)
  ' <<<"${web_state}" >/dev/null

deployment_complete=true
trap - EXIT
echo "Deployment tuple ${DEPLOYMENT_REVISION} is live in ${K8S_NAMESPACE}"
