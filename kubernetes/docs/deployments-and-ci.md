# Deployments and CI

GitHub Actions changes live images for three Deployments only:
`backend-api`, `backend-worker`, and `web`. Kustomize and an administrator's
`kubectl` apply manage every other workload. There is no in-cluster GitOps
controller.

## Staging on every push to `main`

`.github/workflows/publish-main-images.yml` starts on each push to `main`.
It builds multi-platform images for the Go backend and Next.js web application,
then publishes these tags to GHCR:

```text
ghcr.io/usehivy/hivy:main
ghcr.io/usehivy/hivy:dev
ghcr.io/usehivy/hivy:sha-GIT_SHA

ghcr.io/usehivy/web:main
ghcr.io/usehivy/web:dev
ghcr.io/usehivy/web:sha-GIT_SHA
```

The same workflow builds `ghcr.io/usehivy/msb` and
`ghcr.io/usehivy/microsandbox-gateway`, but it does not deploy those two images.
Their Kubernetes manifests remain pinned until an operator changes and applies
them.

After both backend and web builds finish, `deploy-staging` takes their registry
digests, opens a restricted SSH tunnel to the Kubernetes API, patches all three
staging Deployments, and polls each Deployment for up to ten minutes. The
backend digest goes into the API and worker init containers as well as their
main containers. The job succeeds only when every desired replica has updated
and become available.

A push to `main` is the staging trigger. In GitHub, open the
`publish-main-images` run for that commit and check these jobs:

1. `Go API image`
2. `Next.js web image`
3. `Deploy staging application images`

The Microsandbox image jobs run beside them, but they do not gate or alter the
three application Deployments.

## Production on a stable release

`.github/workflows/release.yml` starts when a GitHub Release is published or
when an operator supplies a tag through `workflow_dispatch`. Tags must match
`vX.Y.Z` or `vX.Y.Z-suffix`, and the tagged commit must belong to `main`.

A stable `vX.Y.Z` release builds backend and web images tagged with the full
tag, version without `v`, and `latest`. The `deploy-production` job patches
production with immutable digests and waits using the same procedure as
staging.

A tag containing a suffix, such as `v7.3.0-rc.1`, counts as a prerelease and
does not deploy production. The decision comes from the tag string, not the
GitHub Release's prerelease checkbox. Publishing a release named `v7.3.0` will
deploy production even if someone marks it as a prerelease in GitHub.

The release workflow also builds sandbox runtime and app images, writes a
`release-manifest.json` asset, and publishes Daytona snapshots. Those jobs do
not update the Kubernetes Microsandbox control or gateway Deployments. A legacy
job named `update-railway-runtime-config` still exists for stable releases; its
presence does not update Kubernetes runtime configuration.

Publish a normal production release with GitHub CLI:

```sh
git tag vX.Y.Z COMMIT_ON_MAIN
git push origin vX.Y.Z
gh release create vX.Y.Z --verify-tag --generate-notes
```

Or dispatch the workflow for an existing tag:

```sh
gh workflow run release.yml -f tag=vX.Y.Z
```

GitHub Environment protection rules, if configured in repository settings,
run before the `staging` or `production` deployment job. They are external to
this repository, so the manifests cannot prove that an approval rule exists.

## Private Kubernetes API access

The Kubernetes API is not exposed as a general public endpoint. Each deployment
job tries the addresses in `K8S_TUNNEL_HOSTS` and forwards a local port to
`127.0.0.1:6443` on the first reachable K3s server. SSH checks pinned host keys.

Ansible creates two system users on every K3s server:

```text
hivy-deploy-staging
hivy-deploy-production
```

They have `/usr/sbin/nologin`, locked passwords, and an `authorized_keys` rule
that permits forwarding only to `127.0.0.1:6443`. The SSH tunnel grants network
reachability, not Kubernetes permissions.

Each namespace contains its own `github-actions-deployer` ServiceAccount. Its
Role can only `get` and `patch` the named `backend-api`, `backend-worker`, and
`web` Deployments in that namespace. It cannot list Deployments, inspect Pods,
read Secrets or ConfigMaps, touch data services, or modify cluster-wide objects.

Set these entries in both GitHub Environments:

| Name | Type | Contents |
| --- | --- | --- |
| `KUBE_CONFIG_B64` | Secret | Base64 namespace-scoped ServiceAccount kubeconfig |
| `K8S_TUNNEL_SSH_KEY_B64` | Secret | Base64 private Ed25519 key for that environment's tunnel user |
| `K8S_TUNNEL_KNOWN_HOSTS_B64` | Secret | Base64 pinned SSH host-key file for every tunnel host |
| `K8S_TUNNEL_HOSTS` | Variable | Space-separated K3s server public addresses, in failover order |
| `K8S_TUNNEL_USER` | Variable | `hivy-deploy-staging` or `hivy-deploy-production` |

The local source files live under `kubernetes/config/kubeconfigs/github-actions/`
and `kubernetes/config/credentials/github-actions/`. They are ignored by Git.
Reconcile the restricted server accounts after adding a K3s node:

```sh
cd ansible
ansible-playbook playbooks/k3s/deploy-tunnel.yml
```

## What an automated deployment changes

`scripts/deploy/kubernetes-images.sh` rejects mutable tags. Both supplied image
references must use `@sha256:...`. It then patches:

| Deployment | Fields changed |
| --- | --- |
| `backend-api` | `initContainers[migrate].image`, `containers[api].image` |
| `backend-worker` | `initContainers[migrate].image`, `containers[worker].image` |
| `web` | `containers[web].image` |

It does not apply manifests, change replica counts, update Nango, rotate
configuration, or deploy Microsandbox components. The image digests in each
`kustomization.yaml` are static records and are not rewritten by CI. Applying a
full environment overlay later can therefore restore those recorded digests.
Before an intentional full apply, update the overlay digests to the version
that should remain live.

## Apply an environment from manifests

Use phased applies for a new namespace or a rebuilt data plane. Replace
`ENVIRONMENT` with `staging` or `production`:

```sh
kubernetes/config/validate.sh

kubectl apply -k kubernetes/environments/ENVIRONMENT \
  --selector=hivy.io/apply-phase=foundation
kubectl apply -k kubernetes/environments/ENVIRONMENT \
  --selector=hivy.io/apply-phase=data

kubectl apply -f \
  kubernetes/config/generated/k8s0/apps/qdrant-ENVIRONMENT-v1.18.2.yaml

kubectl wait -n ENVIRONMENT --for=condition=Ready \
  cluster.postgresql.cnpg.io/backend-postgres --timeout=10m
kubectl wait -n ENVIRONMENT --for=condition=Ready \
  cluster.postgresql.cnpg.io/nango-postgres --timeout=10m
kubectl rollout status -n ENVIRONMENT statefulset/qdrant --timeout=10m

kubectl apply -k kubernetes/environments/ENVIRONMENT \
  --selector=hivy.io/apply-phase=workload
```

For production, install or update the Zot Helm release after the namespace
exists, then apply the production `ingress` phase so its private exposure and
certificate match the release:

```sh
helm upgrade --install zot oci://ghcr.io/project-zot/helm-charts/zot \
  --version 0.1.116 \
  --namespace production \
  --values kubernetes/environments/production/zot-values.yaml \
  --wait --timeout 10m

kubectl apply -k kubernetes/environments/production \
  --selector=hivy.io/apply-phase=ingress
```

Qdrant is outside both Kustomize overlays, and Zot is a Helm release. A plain
`kubectl apply -k kubernetes/environments/ENVIRONMENT` does not reconcile
either one. The phased commands also do not wait for Redis topology health;
check its custom resource before starting API and worker Pods.

## Verify a deployment

Use an administrator kubeconfig from `kubernetes/config/kubeconfigs/`, not the
limited CI account, for operational inspection:

```sh
export KUBECONFIG="$PWD/kubernetes/config/kubeconfigs/k8s0/local.yaml"

kubectl get deployment -n staging backend-api backend-worker web \
  -o custom-columns='NAME:.metadata.name,DESIRED:.spec.replicas,READY:.status.readyReplicas,IMAGE:.spec.template.spec.containers[0].image'

kubectl get deployment -n production backend-api backend-worker web \
  -o custom-columns='NAME:.metadata.name,DESIRED:.spec.replicas,READY:.status.readyReplicas,IMAGE:.spec.template.spec.containers[0].image'

kubectl rollout status -n staging deployment/backend-api --timeout=10m
kubectl rollout status -n staging deployment/backend-worker --timeout=10m
kubectl rollout status -n staging deployment/web --timeout=10m
```

Check the migration image separately when diagnosing a rollout:

```sh
kubectl get deployment -n staging backend-api \
  -o jsonpath='{.spec.template.spec.initContainers[?(@.name=="migrate")].image}{"\n"}'
```

Then test the public readiness surfaces:

```sh
curl --fail --show-error https://staging.api.usehivy.com/healthz
curl --fail --show-error https://staging.usehivy.com/api/health
```

Use production hostnames only after the production workflow reports that all
three rollouts completed.

## Roll back application images

Deployment history keeps three revisions. Roll back API and worker together
because they share one image and database migration set:

```sh
kubectl rollout history -n ENVIRONMENT deployment/backend-api
kubectl rollout history -n ENVIRONMENT deployment/backend-worker
kubectl rollout history -n ENVIRONMENT deployment/web

kubectl rollout undo -n ENVIRONMENT deployment/backend-api --to-revision=REVISION
kubectl rollout undo -n ENVIRONMENT deployment/backend-worker --to-revision=REVISION
kubectl rollout undo -n ENVIRONMENT deployment/web --to-revision=REVISION

kubectl rollout status -n ENVIRONMENT deployment/backend-api --timeout=10m
kubectl rollout status -n ENVIRONMENT deployment/backend-worker --timeout=10m
kubectl rollout status -n ENVIRONMENT deployment/web --timeout=10m
```

Replace `ENVIRONMENT` with `staging` or `production`. A Kubernetes image rollback
does not reverse Goose migrations. Before rolling back across a schema change,
confirm that the older binary can run against the migrated database. If it
cannot, deploy a forward-compatible repair rather than running a blind
migration downgrade.

After rollback, copy the selected digests into the environment Kustomization;
otherwise the next full `kubectl apply -k` can change them again. A later push
to `main` replaces a staging rollback, and the next stable release replaces a
production rollback.

## Failure locations

If image build fails, nothing changes in the cluster. If SSH setup fails, check
the ordered tunnel hosts, pinned host keys, environment-specific user, and the
base64 values before touching Kubernetes RBAC. A successful tunnel followed by
`Forbidden` means the ServiceAccount credential or Role is wrong.

If patching succeeds but rollout polling times out, inspect the new ReplicaSet,
Pod events, init-container migrations, and readiness logs:

```sh
kubectl get replicaset,pod -n ENVIRONMENT \
  -l app.kubernetes.io/part-of=hivy -o wide
kubectl describe deployment -n ENVIRONMENT backend-api
kubectl logs -n ENVIRONMENT POD_NAME -c migrate
kubectl logs -n ENVIRONMENT POD_NAME --all-containers=true
```

Do not rerun a production release repeatedly until the failing Pod explains why
it is unready; rebuilding the same digest will not repair a database, Secret, or
dependency failure.
