# Staging backend review package

This Kustomize overlay defines the isolated staging backend and its data plane.
Resources are separated into explicit apply phases so dependencies can be
reviewed and brought up before the API and workers.

## Contents

- namespace `staging`, with a bounded resource quota and default limits;
- one single-instance CloudNativePG PostgreSQL 18.4 database with a 10 GiB
  expandable Longhorn volume;
- one persistent standalone Redis 8.6 instance with a 2 GiB Longhorn volume;
- one Qdrant 1.18.2 instance with replication factor one, API-key
  authentication, and a 1 GiB expandable Longhorn volume;
- native daily PostgreSQL base backups with continuous WAL archiving, daily
  Redis RDB exports, and daily Qdrant snapshots in Hetzner
  Object Storage;
- one API replica, exposed only through the shared Gateway and internally by a
  ClusterIP Service;
- one Asynq worker replica with no Service and ingress denied;
- one Next.js web replica, exposed through the shared Gateway and configured to
  proxy server-side API traffic over the cluster-local backend Service;
- a migration init container on every API and worker Pod, serialized by Goose's
  PostgreSQL advisory lock;
- HTTPRoutes for `staging.usehivy.com`, `staging.api.usehivy.com`,
  `staging.mcp.usehivy.com`, and `staging.proxy.usehivy.com`.

The imported database is a production snapshot. Cryptographic, authentication,
OAuth, delivery, and Nango credentials therefore match production so encrypted
rows and existing sessions remain usable. Paystack stays in test mode, and the
PostgreSQL, Redis, Qdrant, and application-storage credentials remain isolated.
All plaintext secret inputs are ignored by Git.

## Known prerequisites

The backend treats Nango as a hard startup dependency and fetches its provider
catalog through the private `nango:3003` Service during bootstrap. Nango has its
own single-instance PostgreSQL database and a public HTTPS OAuth callback domain.

RAG uses the internal Qdrant gRPC Service at
`qdrant.staging.svc.cluster.local:6334`. The Qdrant authentication Secret and
NetworkPolicy are included in the Kustomize data phase; Qdrant itself is
rendered from the official Helm chart and applied as a separate manifest. The
backend workload phase must wait for the StatefulSet to become Ready.

Staging runs one Qdrant process capped at 1 GiB of memory with a 1 GiB
Longhorn volume. New collections default to two shards, replication factor
one, and write consistency factor one. Production continues to use three
peers with a 10 GiB memory cap per peer.

Staging and production share the Microsandbox control plane, runners, image
versions, and current control API token. The backend reaches the control plane
through the internal `microsandbox-control.production.svc.cluster.local`
Service; there is no `msb.usehivy.com` dependency. Staging begins from the
requested production database snapshot and shares continuity secrets, while
Paystack, application storage, and Kubernetes data-plane credentials remain
environment-specific.

Sandbox runtime URLs must keep using `*.preview.usehivy.com` because browsers
connect to them directly for streams and repository reads. Private transport is
provided by split-horizon DNS: public clients resolve the wildcard to the
public Gateway, while cluster Pods resolve it to the production preview TLS
bridge. The bridge prepends PROXY v2 for Cilium, after which the preview proxy
reaches runner host ports over the Hetzner vSwitch. The
Microsandbox manifests must include this CoreDNS/private-preview path before
the backend is deployed.

Production PostgreSQL workloads use three-instance CloudNativePG clusters.
Staging deliberately uses one instance per database to conserve resources and
therefore has no database failover.

The shared Cilium Gateway serves the staging API over public and private HTTPS.
HTTP redirects to HTTPS, and cert-manager's DNS-01 certificate includes
`staging.api.usehivy.com` and `staging.usehivy.com`.

## Secret inputs

Generate or refresh backend continuity secrets from Railway, the production
Microsandbox control Secret, and the environment-specific Hetzner bucket:

```sh
kubernetes/environments/generate-backend-secrets.sh --refresh
```

Generate the ignored web Secret input separately. Production reuses its
Railway session secret for login continuity; staging receives an independent
session secret:

```sh
kubernetes/environments/generate-web-secrets.sh
```

The script refuses to overwrite existing files. Secret rotation is deliberate:
update the ignored input file, apply the generated Secret, then restart the API
and worker Deployments. If the Qdrant key changed, restart the Qdrant StatefulSet
as well. Kustomize name hashes are disabled because operator CRDs and the Qdrant
chart refer to fixed Secret names.

If the PostgreSQL, Redis, and backend secret files were generated before
Qdrant was added, generate only the missing Qdrant input without rotating the
others:

```sh
kubernetes/environments/staging/secrets/generate-qdrant.sh
```

Generate or refresh the ignored staging backup inputs from the repository-root
`.env.hetzner-s3` file:

```sh
kubernetes/environments/generate-data-secrets.sh --refresh-backups
```

Backup schedules, retention, restore checks, and disaster-recovery caveats are
documented in `../BACKUPS.md`.

Add the required external credentials listed in `CREDENTIALS.md`, then run:

```sh
kubernetes/environments/staging/secrets/validate.sh
```

The validator intentionally blocks deployment when any production feature
would otherwise be silently disabled. It also rejects a live Paystack key.

## Review and render

Render to a protected temporary file so generated Secret values are never
printed to the terminal:

```sh
umask 077
rendered="$(mktemp)"
kubectl kustomize kubernetes/environments/staging > "$rendered"
kubectl apply --dry-run=client -f "$rendered"
rm -f "$rendered"
```

The pinned backend image is commit
`5f93ab690c85fd277ace24e45048ec328f21ff77`, linux/amd64 manifest digest
`sha256:59071935749eebb9b79634e96b7f880e29e0b75e5d799bab1c25e4623bed880c`.
Every newly created API or worker Pod runs the migrations embedded in that
same image before its application process starts. Concurrent Pod starts are
safe because the migration command uses Goose's PostgreSQL session lock.

Qdrant uses the unprivileged 1.18.2 image pinned to multi-architecture manifest
digest
`sha256:b79aaa49ce7a7e5b7e9cf3fe76be400c911457084b4b7af47487c1c9ae5962e5`.
It is rendered from the official `qdrant/qdrant-helm` chart version 1.18.2.
Helm is used only as a renderer; the resulting namespaced manifest is applied
directly with `kubectl`. Download and verify the chart, then render it into the
ignored secrets directory:

```sh
mkdir -p ansible/.secrets/k8s0/apps
curl --fail --location \
  https://github.com/qdrant/qdrant-helm/releases/download/qdrant-1.18.2/qdrant-1.18.2.tgz \
  -o ansible/.secrets/k8s0/apps/qdrant-1.18.2.tgz
printf '%s  %s\n' \
  7ccdd09343a4b6f6546309e2d0ca31f8034f83883fd9362388d19b2a74fdcea0 \
  ansible/.secrets/k8s0/apps/qdrant-1.18.2.tgz | shasum -a 256 -c -

helm template qdrant ansible/.secrets/k8s0/apps/qdrant-1.18.2.tgz \
  --namespace staging \
  --no-hooks \
  --values kubernetes/environments/staging/qdrant-values.yaml \
  --post-renderer kubernetes/environments/staging/qdrant-post-render.sh \
  > ansible/.secrets/k8s0/apps/qdrant-staging-v1.18.2.yaml
chmod 600 ansible/.secrets/k8s0/apps/qdrant-staging-v1.18.2.yaml
```

The post-renderer explicitly adds `metadata.namespace: staging` and replaces
the tag selected by the chart with the pinned image digest. This prevents a raw
`kubectl apply -f` from accidentally creating chart resources in `default`.

## Apply sequence after approval

The labels make the Kustomize dependency order explicit. Apply the Qdrant
authentication and policy first, then validate and apply the separately
rendered chart manifest:

```sh
kubectl apply -k kubernetes/environments/staging \
  --selector=hivy.io/apply-phase=foundation
kubectl apply -k kubernetes/environments/staging \
  --selector=hivy.io/apply-phase=data

kubectl wait -n staging --for=condition=Ready \
  cluster.postgresql.cnpg.io/backend-postgres --timeout=10m
kubectl rollout status -n staging \
  statefulset/backend-redis --timeout=10m
kubectl apply --dry-run=server \
  -f ansible/.secrets/k8s0/apps/qdrant-staging-v1.18.2.yaml
kubectl apply \
  -f ansible/.secrets/k8s0/apps/qdrant-staging-v1.18.2.yaml
kubectl rollout status -n staging statefulset/qdrant --timeout=10m

kubectl apply -k kubernetes/environments/staging \
  --selector=hivy.io/apply-phase=workload
```
