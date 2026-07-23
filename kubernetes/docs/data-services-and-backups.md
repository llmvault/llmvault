# Data services, storage, and backups

Stateful workloads use Longhorn volumes, but each database owns its backup
format. A replicated block volume protects against one disk or node failure; it
doesn't replace PostgreSQL WAL, Redis RDB files, or Qdrant collection snapshots.

## Object storage inventory

All buckets use Hetzner Object Storage in `hel1`. The application buckets hold
user-facing objects; the remaining buckets hold native backup formats:

| Bucket | Writer and contents |
| --- | --- |
| `usehivy-staging-app` | Staging API uploads and signed objects |
| `usehivy-prod-app` | Production API uploads and signed objects |
| `usehivy-staging-pg-backend` | Staging backend PostgreSQL under `postgres/`; staging Redis under `redis/` |
| `usehivy-prod-pg-backend` | Production backend PostgreSQL under `postgres/`; production Redis under `redis/` |
| `usehivy-staging-pg-nango` | Staging Nango PostgreSQL |
| `usehivy-prod-pg-nango` | Production Nango PostgreSQL |
| `usehivy-prod-pg-microsandbox` | Production Microsandbox PostgreSQL |
| `usehivy-staging-qdrant` | Staging Qdrant collection snapshots |
| `usehivy-prod-qdrant` | Production Qdrant peer snapshots |
| `usehivy-k3s-etcd` | K3s embedded-etcd snapshots |

Each service reads its own ignored credential file. Do not give an application
upload identity access to backup buckets or reuse a backup identity between
unrelated buckets.

## Storage layer

Longhorn `v1.12.0` supplies the default `longhorn` StorageClass. New volumes use
two replicas, best-effort data locality, `ext4`, and delayed binding. Longhorn
keeps 15 percent of each default disk free and doesn't over-provision beyond
the disk's reported capacity. Its V2 data engine is disabled.

Both K3s servers contribute `/var/lib/longhorn` from their local NVMe RAID1.
With two Kubernetes nodes, a two-replica volume places one copy on each node.
Adding a third node doesn't change existing volume replica counts; raise the
count deliberately and wait for `healthy` before relying on the new copy.

Longhorn's chart values live in
[`../operators/longhorn/values.yaml`](../operators/longhorn/values.yaml). The
host needs `iscsi_tcp`, `nfs`, and `dm_crypt`; `iscsid` must run, while the
unused multipath daemon stays masked.

## PostgreSQL

CloudNativePG `v1.30.0` manages PostgreSQL `18.4`. Each application gets a
separate cluster and credentials even though all clusters share one operator.

| Namespace | Cluster | Instances | Volume per instance | Backup retention |
| --- | --- | ---: | ---: | ---: |
| production | `backend-postgres` | 3 | 50 GiB | 30 days |
| production | `nango-postgres` | 3 | 10 GiB | 30 days |
| production | `microsandbox-postgres` | 1 | 3 GiB | 30 days |
| staging | `backend-postgres` | 1 | 10 GiB | 14 days |
| staging | `nango-postgres` | 1 | 5 GiB | 14 days |

Production backend and Nango can promote a streaming replica. Microsandbox and
both staging databases cannot fail over because they have one PostgreSQL
instance. Longhorn still keeps two block replicas for their volumes.

The Barman Cloud plugin `v0.13.0` archives WAL continuously and creates base
backups on these schedules. West Africa Time (WAT) is UTC+1:

| Cluster | Schedule | WAT |
| --- | --- | --- |
| production backend | 01:00 daily | 02:00 |
| production Microsandbox | 02:00 daily | 03:00 |
| production Nango | 02:30 daily | 03:30 |
| staging backend | 02:00 daily | 03:00 |
| staging Nango | 02:30 daily | 03:30 |

The corresponding `ObjectStore` resources write to dedicated Hetzner Object
Storage buckets under `postgres/`. PostgreSQL backup credentials never share
the application's S3 identity.

Check cluster health and backup state with:

```sh
kubectl get clusters.postgresql.cnpg.io -A
kubectl get scheduledbackups.postgresql.cnpg.io -A
kubectl get backups.postgresql.cnpg.io -A
kubectl -n production describe cluster.postgresql.cnpg.io/backend-postgres
kubectl -n cnpg-system logs deployment/cnpg-controller-manager --since=30m
kubectl -n cnpg-system logs deployment/barman-cloud --since=30m
```

## Redis

The OT Redis Operator `v0.26.0` owns both environments. Both run the same pinned
Redis `8.6.2` image. Production uses a Redis Cluster with three leaders and
three followers. Each Pod requests a 5 GiB data volume plus a 256 MiB
node-configuration volume. Staging uses one standalone Redis process with a
2 GiB volume.

PVC retention is `Retain` when the Redis resource gets deleted or scaled. That
protects the PVC from an operator action, but it also means an operator won't
clean up abandoned claims for you.

```sh
kubectl -n production get rediscluster.redis.redis.opstreelabs.in/backend-redis
kubectl -n staging get redis.redis.redis.opstreelabs.in/backend-redis
kubectl -n redis-operator logs deployment/redis-operator --since=30m
kubectl -n production get pods,pvc -l app.kubernetes.io/name=backend-redis
```

The `backend-redis-backup` CronJob runs at 03:00 UTC each day with
`timeZone: Etc/UTC`. Staging uploads `standalone.rdb`; production uploads an RDB
from every leader and records the cluster topology. Before upload, the job
checks each RDB structurally, boots a disposable loopback Redis with a bounded
readiness wait, and rejects a production snapshot set if cluster health or slot
topology changes during export. The upload step verifies the remote size of
every expected object and includes `manifest.sha256` for restore verification.
Both environments use the backend PostgreSQL bucket under a separate `redis/`
prefix.

## Qdrant

Qdrant `1.18.2` comes from the official `qdrant/qdrant-helm` chart. Helm only
renders YAML; `kubectl` owns the resulting StatefulSet and related objects.
The post-renderer sets the namespace and replaces the chart tag with the pinned
image digest.

| Namespace | Peers | Volume per peer | Memory limit | Collection replication |
| --- | ---: | ---: | ---: | ---: |
| production | 3 | 10 GiB | 10 GiB | 2 |
| staging | 1 | 1 GiB | 1 GiB | 1 |

The service stays cluster-local on ports `6333` (HTTP), `6334` (gRPC), and
`6335` (peer traffic). API-key authentication is mandatory. NetworkPolicy only
admits the backend and backup job paths declared by the environment.

Render and apply the pinned manifests from the repository root:

```sh
mkdir -p kubernetes/config/generated/k8s0/apps
curl --fail --location \
  https://github.com/qdrant/qdrant-helm/releases/download/qdrant-1.18.2/qdrant-1.18.2.tgz \
  -o kubernetes/config/generated/k8s0/apps/qdrant-1.18.2.tgz

printf '%s  %s\n' \
  7ccdd09343a4b6f6546309e2d0ca31f8034f83883fd9362388d19b2a74fdcea0 \
  kubernetes/config/generated/k8s0/apps/qdrant-1.18.2.tgz | shasum -a 256 -c -

helm template qdrant kubernetes/config/generated/k8s0/apps/qdrant-1.18.2.tgz \
  --namespace staging \
  --no-hooks \
  --values kubernetes/environments/staging/qdrant-values.yaml \
  --post-renderer kubernetes/environments/staging/qdrant-post-render.sh \
  > kubernetes/config/generated/k8s0/apps/qdrant-staging-v1.18.2.yaml

kubectl apply -f \
  kubernetes/config/generated/k8s0/apps/qdrant-staging-v1.18.2.yaml
kubectl rollout status -n staging statefulset/qdrant --timeout=10m
```

Change `staging` to `production` and use the production values and rendered
filename for production. Don't run the chart as a Helm release; that would add
a second owner for objects currently managed by direct apply.

The `qdrant-backup` CronJob starts at 02:30 UTC daily. Staging creates one
collection snapshot. Production asks every peer for a snapshot, since one peer
snapshot doesn't represent every shard in a distributed collection.

## Backup verification

Start a one-off Redis or Qdrant backup without changing the CronJob:

```sh
kubectl create job -n staging --from=cronjob/backend-redis-backup \
  backend-redis-backup-manual
kubectl wait -n staging --for=condition=complete \
  job/backend-redis-backup-manual --timeout=10m
kubectl logs -n staging job/backend-redis-backup-manual

kubectl create job -n staging --from=cronjob/qdrant-backup \
  qdrant-backup-manual
kubectl wait -n staging --for=condition=complete \
  job/qdrant-backup-manual --timeout=10m
kubectl logs -n staging job/qdrant-backup-manual
```

A successful upload proves only that the writer reached S3. Restore tests prove
that the bytes work:

- Recover PostgreSQL into a new one-instance CloudNativePG cluster, wait for
  the required ending WAL, then query a known row.
- Restore Redis into a disposable Redis instance and inspect keys plus TTLs.
- Restore Qdrant under a temporary collection name and query a known point. For
  production, use the matching snapshot set from all peers.
- Never make the first restore attempt against the live cluster or collection.

VictoriaMetrics alerts when a Redis backup Job fails or when the most recent
successful Redis recovery point is older than 30 hours. These alerts still
require an external Alertmanager receiver before they can page an operator.

K3s handles etcd separately. Each server takes a snapshot every six hours and
uploads it to `usehivy-k3s-etcd` through K3s's S3 support. Its credentials and
configuration live under `kubernetes/config/credentials/k3s/`.

## Zot storage and garbage collection

Zot `2.1.17` starts with one expandable 10 GiB Longhorn PVC. Content
deduplication and garbage collection are enabled. Garbage collection runs every
24 hours and waits one hour before removing unreferenced blobs.

Garbage collection does not choose old tags for deletion. Remove obsolete
image manifests or tags through an authenticated registry maintenance process,
keep every digest referenced by the live runtime config and rollback policy,
then let Zot collect the unreferenced layers. Test a staging sandbox pull before
deleting a production rollback image. Runner image prewarming is not part of
the current deployment process.

Operator downloads, checksums, host prerequisites, render commands, and smoke
checks are in [Storage and data operators](operators.md).
