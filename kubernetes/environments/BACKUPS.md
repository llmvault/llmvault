# Data backup runbook

Backups are application-native and stored in Hetzner Object Storage. Longhorn
volume snapshots are not the database backup mechanism.

## Schedules and retention

| Service | Staging | Production | Destination |
| --- | --- | --- | --- |
| PostgreSQL base backup | daily 03:00 WAT (02:00 UTC) | daily 02:00 WAT (01:00 UTC) | environment backend PostgreSQL bucket under `postgres/` |
| PostgreSQL WAL | continuous | continuous | same bucket under `postgres/` |
| Redis Cluster RDB | daily 04:00 WAT (03:00 UTC) | daily 04:00 WAT (03:00 UTC) | environment backend PostgreSQL bucket under `redis/` |
| Qdrant peer snapshots | daily 03:30 WAT (02:30 UTC) | daily 03:30 WAT (02:30 UTC) | environment Qdrant bucket |
| K3s etcd | every 6 hours | shared control plane | `usehivy-k3s-etcd` |

PostgreSQL retention is managed by Barman: 14 days in staging and 30 days in
production. Hetzner lifecycle rules expire Redis and Qdrant objects after 14
days in staging and 30 days in production; non-current versions expire after
21 and 45 days respectively. Incomplete multipart uploads expire after 7 days.

## What each backup contains

- CloudNativePG writes compressed base backups and WAL needed for point-in-time
  recovery. A backup is not considered restorable until its required ending WAL
  segment has been archived.
- Redis exports one RDB from every leader, records `CLUSTER NODES`, loads every
  RDB into a temporary Redis process, and verifies the process before upload.
- Qdrant creates a collection snapshot on every peer. Distributed Qdrant
  recovery must restore the matching peer snapshots; a single peer snapshot is
  not a complete distributed backup.
- K3s uploads encrypted-in-transit etcd snapshots through its native S3 support.

## Manual verification

Create a one-off job from either CronJob and inspect its result:

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

For PostgreSQL, create a `Backup` using method `plugin` and plugin name
`barman-cloud.cloudnative-pg.io`, wait for `status.phase: completed`, confirm the
ending WAL exists, and recover into a temporary one-instance CloudNativePG
Cluster. Query a known canary row before declaring the backup verified.

For Qdrant, download each peer snapshot, upload each one to the corresponding
peer under a temporary collection name with `priority=snapshot`, and query a
known canary point. Delete only the temporary collection after validation.

Test restores should be repeated after operator upgrades and at least monthly.
Production recovery must use a new Cluster or collection first; never overwrite
the live data set as the initial restore step.
