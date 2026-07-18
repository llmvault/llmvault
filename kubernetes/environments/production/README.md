# Production data plane

This Kustomize environment contains the production backend data services. It
does not deploy the API, workers, Nango, or Microsandbox workloads.

## Topology

- CloudNativePG PostgreSQL 18.4: one primary and two streaming replicas, with a
  50 GiB Longhorn volume per instance;
- Redis 8.6 Cluster: three leaders and three followers, with a 5 GiB data
  volume and 256 MiB node-configuration volume per Pod;
- Qdrant 1.18.2: three peers, with a 10 GiB Longhorn volume and a 10 GiB memory
  limit per peer;
- native daily backups for all three data services, plus continuous PostgreSQL
  WAL archiving.

The production Redis backup override allows the validation process to load a
near-capacity shard and provides 20 GiB of temporary space for all three RDBs.

All Pods currently run on the single Kubernetes node. The replicas protect
against process and Pod failure, but they are not protection against loss of
that node. The anti-affinity and topology rules will distribute peers when more
nodes are added. Longhorn itself is also configured for one storage replica
until another storage-capable node exists.

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

## Apply

```sh
kubectl apply -k kubernetes/environments/production
kubectl apply -f ansible/.secrets/k8s0/apps/qdrant-production-v1.18.2.yaml

kubectl wait -n production --for=condition=Ready \
  cluster.postgresql.cnpg.io/backend-postgres --timeout=10m
kubectl rollout status -n production statefulset/qdrant --timeout=10m
kubectl get rediscluster.redis.redis.opstreelabs.in/backend-redis -n production
```

See `../BACKUPS.md` for schedules and verification procedures.
