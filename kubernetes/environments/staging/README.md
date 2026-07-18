# Staging backend review package

This Kustomize overlay defines the isolated staging backend and its data plane.
Resources are separated into explicit apply phases so dependencies can be
reviewed and brought up before the API and workers.

## Contents

- namespace `staging`, with a bounded resource quota and default limits;
- one CloudNativePG PostgreSQL 18.4 cluster with one primary, two streaming
  replicas, and a separate 10 GiB Longhorn volume per instance;
- a three-leader Redis 8.6 cluster with no followers and persistent Longhorn
  data/node-configuration volumes;
- three cluster-enabled Qdrant 1.18.2 peers, with six collection shards,
  replication factor two, API-key authentication, and a separate 1 GiB
  expandable Longhorn volume per peer;
- two API replicas, exposed only through the shared Gateway and internally by a
  ClusterIP Service;
- two Asynq worker replicas with no Service and ingress denied;
- a migration init container on every API and worker Pod, serialized by Goose's
  PostgreSQL advisory lock;
- HTTPRoutes for `staging.api.usehivy.com`, `staging.mcp.usehivy.com`, and
  `staging.proxy.usehivy.com`.

All application credentials are independent staging or test credentials. The
generated plaintext input files are ignored by Git; only examples, validation,
and generation scripts are committed. See `CREDENTIALS.md` for the external
credential setup checklist.

## Known prerequisites

The backend treats Nango as a hard startup dependency and fetches its provider
catalog during bootstrap. The API and workers will not start until a staging
Nango Service exists at `nango.staging.svc.cluster.local:3003` and uses the same
newly generated Nango credentials as `secrets/backend.env`.

RAG uses the internal Qdrant gRPC Service at
`qdrant.staging.svc.cluster.local:6334`. The Qdrant authentication Secret and
NetworkPolicy are included in the Kustomize data phase; Qdrant itself is
rendered from the official Helm chart and applied as a separate manifest. The
backend workload phase must wait for all three peers to become Ready. Nango is
not part of this package yet, so the API and workers must not be applied until
its staging Service is approved and ready.

Staging runs three Qdrant peers on the current single physical node. Each peer
is capped at 1 GiB of memory and has its own 1 GiB Longhorn volume. New
collections default to six shards, replication factor two, and write
consistency factor one. The replicas protect against a Qdrant process or Pod
failure, but not loss of the physical node; they become node-redundant after
more Kubernetes nodes are added. Production will use a separate values file
with a 10 GiB memory cap per peer.

Staging and production share the Microsandbox control plane, runners, image
versions, and current control API token. The backend reaches the control plane
through the internal `microsandbox-control.microsandbox.svc.cluster.local`
Service; there is no `msb.usehivy.com` dependency. The KMS, JWT, RSA,
sandbox-encryption, PostgreSQL, Redis, Nango, billing, OAuth, and delivery
credentials remain independent from production.

Sandbox runtime URLs must keep using `*.preview.usehivy.com` because browsers
connect to them directly for streams and repository reads. Private transport is
provided by split-horizon DNS: public clients resolve the wildcard to the
public Gateway, while cluster Pods resolve it to the internal preview-proxy
Service. That proxy reaches runner host ports over the Hetzner vSwitch. The
Microsandbox manifests must include this CoreDNS/private-preview path before
the backend is deployed.

All PostgreSQL workloads added later (including Nango and Microsandbox) must
use three-instance CloudNativePG clusters. On the current single node, the
replicas protect against PostgreSQL process/instance failure but not loss of
the physical node; they will become node-redundant when more Kubernetes nodes
are added.

The current `public` Gateway is still the smoke-test Gateway and its listener is
restricted to `gateway-check.usehivy.com`. Before these routes can attach, the
shared Gateway and trusted certificate must be replaced with the real
multi-host ingress configuration. No staging DNS record should be created until
that is ready.

## Secret inputs

Generate the initial independent credentials once:

```sh
kubernetes/environments/staging/secrets/generate.sh
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
`9cd953b24170610d4ddf656524b2b76d313ee652`, manifest digest
`sha256:d310bffaf541e05837a4740f2280f50588bf99816a2e5d889894eab0b7a2cbba`.
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
kubectl wait -n staging --for=jsonpath='{.status.state}'=Ready \
  rediscluster.redis.redis.opstreelabs.in/backend-redis --timeout=10m
kubectl apply --dry-run=server \
  -f ansible/.secrets/k8s0/apps/qdrant-staging-v1.18.2.yaml
kubectl apply \
  -f ansible/.secrets/k8s0/apps/qdrant-staging-v1.18.2.yaml
kubectl rollout status -n staging statefulset/qdrant --timeout=10m

kubectl apply -k kubernetes/environments/staging \
  --selector=hivy.io/apply-phase=workload
```
