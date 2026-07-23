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
grep -Fq "Deploy staging application images" "${main_workflow}"
grep -Fq "K8S_NAMESPACE: staging" "${main_workflow}"
grep -Fq "types: [published]" "${release_workflow}"
grep -Fq "deploy production application images" "${release_workflow}"
grep -Fq "K8S_NAMESPACE: production" "${release_workflow}"

if grep -Eiq "daytona|hivy-sandboxes-runtime-daytona" \
  "${main_workflow}" \
  "${release_workflow}" \
  ".github/workflows/publish-runtime-manifests.yml"; then
  echo "deployment workflows must not publish Daytona images or snapshots" >&2
  exit 1
fi

echo "✓ Staging and production deployment workflows are present; Daytona publication is absent."
