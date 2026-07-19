# Applications and service dependencies

Kustomize builds the application namespaces from reusable bases in
`kubernetes/apps/` and environment overlays in `kubernetes/environments/`.
Staging and production have separate application databases, Redis instances,
Qdrant stores, Nango deployments, Secrets, ConfigMaps, Services, routes, and
resource settings. They share the production Microsandbox control plane and
the two bare-metal sandbox runners.

## Environment boundaries

Both application namespaces enforce the Pod Security `baseline` profile and
audit or warn against `restricted` violations. Each has its own Secrets,
ConfigMaps, Services, policies, and stateful resources; cross-namespace access
is declared only where staging must reach the shared production Microsandbox
control plane.

Staging has a `staging-budget` ResourceQuota: 8 requested CPU cores, 16 GiB of
requested memory, 32 CPU cores and 40 GiB at limits, 128 GiB of requested
storage, 32 PVCs, 60 Pods, and 30 Services. Its LimitRange supplies small
defaults when a container omits requests or limits. Production has default
requests and limits through `production-defaults`, but no ResourceQuota, so its
declared workloads can use the cluster's remaining schedulable capacity.

## Workload inventory

| Workload | Staging | Production | Entry point | Main dependencies |
| --- | ---: | ---: | --- | --- |
| `backend-api` | 1 replica | 2 replicas | `backend-api:8080`; Gateway API routes | backend PostgreSQL, Redis, Qdrant, Nango, Microsandbox control, Hetzner Object Storage, external APIs |
| `backend-worker` | 1 replica | 2 replicas | No Service or ingress; health port `8090` inside its Pod | Same data services and configuration as the API |
| `web` | 1 replica | 2 replicas | `web:8080`; public web route | cluster-local backend API, public Nango and preview URLs |
| `nango` | 1 replica | 1 replica | `nango:3003`; public connections route | its own PostgreSQL cluster |
| `backend-postgres` | 1 instance | 3 instances | `backend-postgres-rw:5432`, namespace-local | backend API and worker |
| `nango-postgres` | 1 instance | 3 instances | `nango-postgres-rw:5432`, namespace-local | Nango |
| `backend-redis` | 1 standalone instance | 3 leaders and 3 followers | `backend-redis:6379` in staging; `backend-redis-leader:6379` in production | backend API and worker |
| `qdrant` | 1 peer | 3 peers | `qdrant:6334` for backend gRPC; `6333` for backup jobs | backend API and worker |

Production also owns the shared sandbox services:

| Workload | Shape | Entry point | Purpose |
| --- | --- | --- | --- |
| `microsandbox-control` | 2 replicas | `microsandbox-control.production.svc.cluster.local:8080`; private NodePort `32080` for runners | Stores runner and sandbox state, schedules sandboxes, and serves the control API |
| `microsandbox-postgres` | 1 CloudNativePG instance | `microsandbox-postgres-rw:5432` | Control-plane database |
| `microsandbox-preview-cache` | 2 replicas | `microsandbox-preview-cache:8091` | Resolves preview hosts and wakes stopped sandboxes through the control API |
| `microsandbox-preview-redis` | 1 ephemeral Redis process | `microsandbox-preview-redis:6379` | Shared preview lookup cache; it does not hold durable application state |
| `microsandbox-preview-proxy` | 2 Caddy replicas | `microsandbox-preview-proxy:8080`; wildcard preview route | Authenticates preview lookup, then proxies to a runner preview port |
| `microsandbox-preview-tls-bridge` | 2 HAProxy replicas | `microsandbox-preview-tls-bridge.production.svc.cluster.local:443` | Keeps in-cluster preview traffic on the private path while retaining HTTPS and the public hostname |
| `zot` | 1 replica | private NodePort `32500`, reached through runner HAProxy on `registry.usehivy.com:5000` | Private sandbox image registry |

Operator-owned services, ingress, storage, backups, and observability live in
separate namespaces. Their ownership and installation procedures are described
in the other files in this directory.

## Backend API and Asynq worker

The API and worker run the same `ghcr.io/usehivy/hivy` image. Command selection
changes its role:

```text
backend-api      /hivy serve
backend-worker   /hivy work
```

Every API and worker Pod first runs `/hivy migrate up` from the same image in a
`migrate` init container. A Pod never starts its main process unless migrations
finish. The deployment workflow therefore updates both the init-container and
application-container image fields together. Keep the API and worker on the
same digest.

The API serves ordinary HTTP traffic on port `8080` and MCP traffic on port
`8081`. A `ClusterIP` Service exposes both ports. The worker exposes no Service,
and a namespace NetworkPolicy denies all inbound connections to worker Pods;
kubelet health probes run directly against port `8090`.

Both processes read `backend-config` and `backend-secrets`, then receive the
database and Redis passwords from their operator-facing Secrets. Nango
credentials come from `nango-backend-credentials`. In each namespace the
backend connects to local PostgreSQL, Redis, Qdrant, and Nango services. Both
environments call the shared control plane at:

```text
http://microsandbox-control.production.svc.cluster.local:8080
```

That cross-namespace call stays inside the cluster. The control-plane policy
accepts callers from both `production` and `staging`.

## Web

Next.js listens on port `8080` behind the `web` ClusterIP Service. Server-side
requests use the private backend address for their own namespace:

```text
http://backend-api.staging.svc.cluster.local:8080
http://backend-api.production.svc.cluster.local:8080
```

Browser code uses the public API, Nango, and preview hostnames because a user's
browser cannot resolve cluster Services. The Pod uses a read-only root
filesystem; writable Next.js cache and `/tmp` directories use bounded
`emptyDir` volumes.

## Nango

Nango runs the Hivy integrations image, `ghcr.io/usehivy/integrations`, on port
`3003`. Each environment has a separate Nango deployment and PostgreSQL
database. The backend reaches it through the namespace-local `nango:3003`
Service, while OAuth redirects and browser flows use the public connections
hostname. Nango runs its own database migrations at startup because
`NANGO_MIGRATE_AT_START=true`.

The API and worker depend on Nango during normal application operation. If the
Nango Pod or its database is unavailable, connection catalog loading and
integration work fail even when the main backend database remains healthy.

## Public hostnames

| Purpose | Staging | Production |
| --- | --- | --- |
| Web application | `staging.usehivy.com` | `usehivy.com` |
| Backend API | `staging.api.usehivy.com` | `api.usehivy.com` |
| Nango | `staging.connections.usehivy.com` | `connections.usehivy.com` |
| LLM proxy | `staging.proxy.usehivy.com` | `proxy.usehivy.com` |
| MCP | `staging.mcp.usehivy.com` | `mcp.usehivy.com` |
| Sandbox previews | Shared `*.preview.usehivy.com` | Shared `*.preview.usehivy.com` |

Plain HTTP redirects to HTTPS for the web, API, Nango, and preview routes. The
LLM proxy routes rewrite `/` to the backend's `/v1/proxy/` prefix. API, proxy,
and MCP routes attach to both the public and private HTTPS Gateway listeners,
which lets runner-local DNS and HAProxy send sandbox traffic through the private
vSwitch without changing application URLs.

## Storage and durability

PostgreSQL, application Redis, Qdrant, and Zot use Longhorn volumes. Staging
uses single-instance data services to save capacity, so it has no service-level
failover for PostgreSQL, Redis, or Qdrant. Production spreads application
replicas and runs clustered data services where the manifests say so; the
Microsandbox PostgreSQL cluster deliberately remains one instance.

Uploads use environment-specific Hetzner Object Storage buckets. Backup jobs
and PostgreSQL WAL archiving use separate bucket credentials from application
uploads. The preview Redis instance has no persistent volume by design.

## Fast dependency checks

Start with workload readiness, then follow the failed dependency instead of
restarting everything:

```sh
kubectl get pods -n staging
kubectl get pods -n production
kubectl get httproutes -A

kubectl get cluster.postgresql.cnpg.io -n staging
kubectl get cluster.postgresql.cnpg.io -n production
kubectl get redis.redis.opstreelabs.in -n staging
kubectl get rediscluster.redis.redis.opstreelabs.in -n production
kubectl get statefulset qdrant -n staging
kubectl get statefulset qdrant -n production
```

For one failed Pod, inspect its events and every container, including migration
init containers:

```sh
kubectl describe pod -n staging POD_NAME
kubectl logs -n staging POD_NAME --all-containers=true
kubectl logs -n staging POD_NAME -c migrate
```

Use the equivalent `production` commands for production. A backend Pod stuck in
`Init:*` points to migrations or PostgreSQL connectivity; a running but unready
Pod points to `/readyz` dependencies or application startup.
