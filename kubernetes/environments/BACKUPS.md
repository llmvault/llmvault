# Data backup runbook

Backups are application-native and stored in Hetzner Object Storage. Longhorn
volume snapshots are not the database backup mechanism.

## Schedules and retention

| Service | Staging | Production | Destination |
| --- | --- | --- | --- |
| PostgreSQL base backup | daily 03:00 WAT (02:00 UTC) | daily 02:00 WAT (01:00 UTC) | environment backend PostgreSQL bucket under `postgres/` |
| PostgreSQL WAL | continuous | continuous | same bucket under `postgres/` |
| Redis RDB | daily 04:00 WAT (03:00 UTC) | daily 04:00 WAT (03:00 UTC) | environment backend PostgreSQL bucket under `redis/` |
| Qdrant snapshots | daily 03:30 WAT (02:30 UTC) | daily 03:30 WAT (02:30 UTC) | environment Qdrant bucket |
| K3s etcd | every 6 hours | shared control plane | `usehivy-k3s-etcd` |

PostgreSQL retention is managed by Barman: 14 days in staging and 30 days in
production. Hetzner lifecycle rules expire Redis and Qdrant objects after 14
days in staging and 30 days in production; non-current versions expire after
21 and 45 days respectively. Incomplete multipart uploads expire after 7 days.

## What each backup contains

- CloudNativePG writes compressed base backups and WAL needed for point-in-time
  recovery. A backup is not considered restorable until its required ending WAL
  segment has been archived.
- Staging exports and validates one standalone Redis RDB. Production exports
  one RDB from every cluster leader and also records `CLUSTER NODES`.
- Staging creates one Qdrant collection snapshot. Production creates a
  collection snapshot on every peer; distributed recovery must restore the
  matching peer snapshots because one peer snapshot is not a complete backup.
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

For Qdrant, download the staging snapshot or every production peer snapshot,
restore under a temporary collection name with `priority=snapshot`, and query
a known canary point. Delete only the temporary collection after validation.

Test restores should be repeated after operator upgrades and at least monthly.
Production recovery must use a new Cluster or collection first; never overwrite
the live data set as the initial restore step.
