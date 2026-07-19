#!/usr/bin/env bash
set -euo pipefail

: "${K8S_NAMESPACE:?K8S_NAMESPACE is required}"
: "${BACKEND_IMAGE:?BACKEND_IMAGE is required}"
: "${WEB_IMAGE:?WEB_IMAGE is required}"

if [[ ! "${BACKEND_IMAGE}" =~ ^ghcr\.io/usehivy/hivy@sha256:[a-f0-9]{64}$ ]]; then
  echo "BACKEND_IMAGE must be an immutable usehivy/hivy digest" >&2
  exit 1
fi
if [[ ! "${WEB_IMAGE}" =~ ^ghcr\.io/usehivy/web@sha256:[a-f0-9]{64}$ ]]; then
  echo "WEB_IMAGE must be an immutable usehivy/web digest" >&2
  exit 1
fi

kubectl --namespace "${K8S_NAMESPACE}" patch deployment backend-api --type=strategic --patch "$(printf '{\"spec\":{\"template\":{\"spec\":{\"initContainers\":[{\"name\":\"migrate\",\"image\":\"%s\"}],\"containers\":[{\"name\":\"api\",\"image\":\"%s\"}]}}}}' "${BACKEND_IMAGE}" "${BACKEND_IMAGE}")" --output=name
kubectl --namespace "${K8S_NAMESPACE}" patch deployment backend-worker --type=strategic --patch "$(printf '{\"spec\":{\"template\":{\"spec\":{\"initContainers\":[{\"name\":\"migrate\",\"image\":\"%s\"}],\"containers\":[{\"name\":\"worker\",\"image\":\"%s\"}]}}}}' "${BACKEND_IMAGE}" "${BACKEND_IMAGE}")" --output=name
kubectl --namespace "${K8S_NAMESPACE}" patch deployment web --type=strategic --patch "$(printf '{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"web\",\"image\":\"%s\"}]}}}}' "${WEB_IMAGE}")" --output=name

wait_for_rollout() {
  local deployment="$1"
  local deadline=$((SECONDS + 600))

  while ((SECONDS < deadline)); do
    local state generation observed desired updated available unavailable
    state="$(kubectl --namespace "${K8S_NAMESPACE}" get deployment "${deployment}" --output=json)"
    generation="$(jq -r '.metadata.generation' <<<"${state}")"
    observed="$(jq -r '.status.observedGeneration // 0' <<<"${state}")"
    desired="$(jq -r '.spec.replicas // 1' <<<"${state}")"
    updated="$(jq -r '.status.updatedReplicas // 0' <<<"${state}")"
    available="$(jq -r '.status.availableReplicas // 0' <<<"${state}")"
    unavailable="$(jq -r '.status.unavailableReplicas // 0' <<<"${state}")"

    if ((observed >= generation && updated == desired && available == desired && unavailable == 0)); then
      echo "${deployment} rollout complete"
      return 0
    fi
    sleep 5
  done

  echo "${deployment} rollout did not complete within 10 minutes" >&2
  return 1
}

wait_for_rollout backend-api
wait_for_rollout backend-worker
wait_for_rollout web
