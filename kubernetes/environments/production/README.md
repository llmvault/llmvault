# Production environment

This Kustomize environment contains the production backend workloads and data
services.

## Topology

- CloudNativePG PostgreSQL 18.4: one primary and two streaming replicas, with a
  50 GiB Longhorn volume per instance;
- Redis 8.6 Cluster: three leaders and three followers, with a 5 GiB data
  volume and 256 MiB node-configuration volume per Pod;
- Qdrant 1.18.2: three peers, with a 10 GiB Longhorn volume and a 10 GiB memory
  limit per peer;
- Hivy API: two replicas behind the shared Cilium Gateway;
- Asynq workers: two private replicas with no Service or ingress;
- Next.js web: two replicas behind the shared Cilium Gateway, with server-side
  API traffic using the cluster-local backend Service;
- Nango: one application replica backed by a dedicated three-instance
  CloudNativePG cluster;
- Microsandbox control and preview services, shared with staging over private
  cluster networking;
- Zot 2.1.17: one private registry instance with an initial, expandable 10 GiB
  Longhorn volume. Runner-local HAProxy is the only client entry path;
- native daily backups for all three data services, plus continuous PostgreSQL
  WAL archiving.

The production Redis backup override allows the validation process to load a
near-capacity shard and provides 20 GiB of temporary space for all three RDBs.

The cluster currently has two control-plane/worker nodes. Topology rules spread
the web replicas across both nodes; stateful workload placement and Longhorn
replication remain governed by their operator configuration.

## Secrets and rendering

The generated input files under `secrets/` and the rendered Qdrant manifest
under `ansible/.secrets/` are Git-ignored. Generate the initial production data
credentials and all backup inputs with:

```sh
kubernetes/environments/generate-data-secrets.sh
```

Use `--refresh-backups` to update only the S3 inputs without rotating database
credentials. Render Qdrant using the pinned chart and post-renderer in the same
way described by the staging README, changing the namespace, values file, and
output filename to their production counterparts.

Generate or refresh backend continuity secrets copied from Railway with:

```sh
kubernetes/environments/generate-backend-secrets.sh --refresh
```

Generate the web session and observability Secret inputs from the linked
Railway production web service with:

```sh
kubernetes/environments/generate-web-secrets.sh
```

## Apply

```sh
helm upgrade --install zot oci://ghcr.io/project-zot/helm-charts/zot \
  --version 0.1.116 \
  --namespace production \
  --values kubernetes/environments/production/zot-values.yaml \
  --wait --timeout 10m

kubectl apply -k kubernetes/environments/production
kubectl apply -f ansible/.secrets/k8s0/apps/qdrant-production-v1.18.2.yaml

kubectl wait -n production --for=condition=Ready \
  cluster.postgresql.cnpg.io/backend-postgres --timeout=10m
kubectl rollout status -n production statefulset/qdrant --timeout=10m
kubectl get rediscluster.redis.redis.opstreelabs.in/backend-redis -n production
```

The Zot listener is deliberately absent from the Hetzner load balancer. The K3s
host firewall accepts Zot's NodePort `32500` only from runner private IPs, and
each runner resolves `registry.usehivy.com` to its own private HAProxy listener
on port `5000`. Zot uses `externalTrafficPolicy: Local`, so HAProxy sends traffic
only to the healthy node currently hosting the Zot Pod and preserves the runner
source address for its Cilium policy. Zot terminates TLS itself. Add future
runners to the Ansible inventory and to `k3s_private_registry_clients` before
deploying them.

See `../BACKUPS.md` for schedules and verification procedures.
