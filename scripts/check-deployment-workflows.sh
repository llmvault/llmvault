#!/usr/bin/env bash
set -euo pipefail

main_workflow=".github/workflows/publish-main-images.yml"
release_workflow=".github/workflows/release.yml"

for workflow in "${main_workflow}" "${release_workflow}"; do
  if [[ ! -f "${workflow}" ]]; then
    echo "missing required deployment workflow: ${workflow}" >&2
    exit 1
  fi
done

grep -Fq "branches: [main]" "${main_workflow}"
grep -Fq "cancel-in-progress: false" "${main_workflow}"
grep -Fq "Deploy staging application images" "${main_workflow}"
grep -Fq "K8S_NAMESPACE: staging" "${main_workflow}"
grep -Fq 'SANDBOX_RUNTIME_IMAGE_TAG: sha-${{ github.sha }}-amd64' "${main_workflow}"
grep -Fq 'SANDBOX_APP_IMAGE_TAG: sha-${{ github.sha }}-amd64' "${main_workflow}"
grep -Fq "sandbox-runtime" "${main_workflow}"
grep -Fq "sandbox-app" "${main_workflow}"
grep -Fq "types: [published]" "${release_workflow}"
grep -Fq "deploy production application images" "${release_workflow}"
grep -Fq "K8S_NAMESPACE: production" "${release_workflow}"
grep -Fq 'SANDBOX_RUNTIME_IMAGE_TAG: ${{ needs.prepare.outputs.tag }}-amd64' "${release_workflow}"
grep -Fq 'SANDBOX_APP_IMAGE_TAG: ${{ needs.prepare.outputs.tag }}-amd64' "${release_workflow}"
grep -Fq 'HIVY_MICROSANDBOX_RUNNER_FORWARD_TARGETS: ${{ vars.HIVY_MICROSANDBOX_RUNNER_FORWARD_TARGETS }}' "${release_workflow}"
grep -Fq 'HIVY_MICROSANDBOX_RUNNER_API_TOKEN: ${{ secrets.HIVY_MICROSANDBOX_RUNNER_API_TOKEN }}' "${release_workflow}"
grep -Fq "Warm sandbox images on every production runner" "${release_workflow}"

warm_line="$(grep -n "Warm sandbox images on every production runner" "${release_workflow}" | cut -d: -f1)"
deploy_line="$(grep -n "Deploy the release tuple and wait for rollout" "${release_workflow}" | head -1 | cut -d: -f1)"
if ((warm_line >= deploy_line)); then
  echo "production runner images must warm before the deployment rollout" >&2
  exit 1
fi

if grep -Eiq "daytona|hivy-sandboxes-runtime-daytona" \
  "${main_workflow}" \
  "${release_workflow}" \
  ".github/workflows/publish-runtime-manifests.yml"; then
  echo "deployment workflows must not publish Daytona images or snapshots" >&2
  exit 1
fi

bash ./scripts/deploy/kubernetes-images_test.sh
bash ./scripts/deploy/setup-kubernetes-tunnel_test.sh
bash ./scripts/release/warm-microsandbox-runner-images_test.sh

echo "✓ Staging and production deploy coherent image tuples; production runners warm before rollout."
