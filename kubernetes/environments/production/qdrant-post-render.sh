#!/usr/bin/env bash
set -euo pipefail

render_dir="$(mktemp -d)"
trap 'rm -rf "${render_dir}"' EXIT

tee "${render_dir}/resources.yaml" >/dev/null
printf '%s\n' \
  'apiVersion: kustomize.config.k8s.io/v1beta1' \
  'kind: Kustomization' \
  'namespace: production' \
  'resources:' \
  '  - resources.yaml' \
  'images:' \
  '  - name: docker.io/qdrant/qdrant' \
  '    newName: docker.io/qdrant/qdrant' \
  '    digest: sha256:b79aaa49ce7a7e5b7e9cf3fe76be400c911457084b4b7af47487c1c9ae5962e5' \
  > "${render_dir}/kustomization.yaml"

kubectl kustomize "${render_dir}"

