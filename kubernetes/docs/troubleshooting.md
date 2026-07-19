# Troubleshooting runbook

Run commands from the repository root with an administrator kubeconfig unless
a section says otherwise:

```sh
export KUBECONFIG="$PWD/kubernetes/config/kubeconfigs/k8s0/local.yaml"
kubectl cluster-info
kubectl get nodes -o wide
```

If `cluster-info` can't reach `127.0.0.1:16443`, start the SSH tunnel documented
in the bootstrap guide before diagnosing Kubernetes. Don't print Secret data,
environment variables, or kubeconfig contents into a ticket or chat transcript.

## First five minutes

Set the affected namespace, check recent events in time order, and inspect all
workload classes before restarting anything:

```sh
NS=staging
kubectl get pods,deploy,statefulset,job,cronjob,pvc -n "$NS" -o wide
kubectl get events -n "$NS" --sort-by=.lastTimestamp | tail -80
kubectl get httproute -n "$NS"
kubectl get cluster.postgresql.cnpg.io -n "$NS"
kubectl get redis.redis.redis.opstreelabs.in,rediscluster.redis.redis.opstreelabs.in -n "$NS"
```

Look at `STATUS`, `READY`, node placement, restart count, pending PVCs, and the
newest warning event. A restart hides evidence and can move a Pod to another
node, so collect logs and `describe` output first.

Grafana should remain the first log search tool. Open **Hivy / Service Details**,
select the environment and service, and set Search regex to the session UUID or
error text. Use the commands below when Grafana identifies a component or when
the monitoring path itself is broken.

## API

```sh
NS=staging
kubectl get deploy/backend-api svc/backend-api -n "$NS" -o wide
kubectl rollout status deploy/backend-api -n "$NS" --timeout=3m
kubectl get pods -n "$NS" \
  -l app.kubernetes.io/name=hivy-backend,app.kubernetes.io/component=api -o wide
kubectl logs -n "$NS" deploy/backend-api -c api --since=30m --prefix
kubectl describe deploy/backend-api -n "$NS"
```

If a Pod never passes initialization, get its name and read the migration
container rather than the application container:

```sh
POD=$(kubectl get pod -n "$NS" \
  -l app.kubernetes.io/name=hivy-backend,app.kubernetes.io/component=api \
  -o jsonpath='{.items[0].metadata.name}')
kubectl logs -n "$NS" "$POD" -c migrate
kubectl describe pod -n "$NS" "$POD"
```

The API probes `/healthz` and `/readyz` on port `8080`; MCP listens on `8081`.
Test the Service through a local port-forward, which avoids depending on tools
inside an application image:

```sh
kubectl port-forward -n "$NS" service/backend-api 18080:8080
# In a second terminal:
curl --fail --show-error http://127.0.0.1:18080/healthz
```

If the image starts but a request fails, search API logs, then worker logs by
the same session or request identifier. Check PostgreSQL, Redis, Nango, Qdrant,
and Microsandbox only after the first dependency error identifies a path.

## Asynq workers

Workers have no Service and deny all ingress; use Kubernetes logs and their
HTTP health probes on port `8090`.

```sh
NS=production
kubectl rollout status deploy/backend-worker -n "$NS" --timeout=3m
kubectl get pods -n "$NS" \
  -l app.kubernetes.io/name=hivy-backend,app.kubernetes.io/component=worker -o wide
kubectl logs -n "$NS" deploy/backend-worker -c worker --since=30m --prefix
kubectl describe deploy/backend-worker -n "$NS"
```

A worker stuck in `Init` uses the same `migrate` init container as the API.
Read that container's logs. For a clean worker Pod with no task activity, check
Redis reachability and whether the API enqueued the task; absence of worker
errors doesn't prove a task entered Redis.

## Web

```sh
NS=staging
kubectl rollout status deploy/web -n "$NS" --timeout=5m
kubectl get pods -n "$NS" \
  -l app.kubernetes.io/name=hivy-web,app.kubernetes.io/component=frontend -o wide
kubectl logs -n "$NS" deploy/web -c web --since=30m --prefix
kubectl describe httproute web -n "$NS"
curl --resolve staging.usehivy.com:443:65.109.40.68 \
  -I https://staging.usehivy.com/
```

The readiness endpoint is `/api/health` on port `8080`. Browser calls use the
public API URL, while Next.js server-side traffic uses the internal backend
Service configured in `web.config.env`. If the page loads but server rendering
fails, compare web logs with API logs at the same second.

## Nango

```sh
NS=staging
kubectl rollout status deploy/nango -n "$NS" --timeout=5m
kubectl get pods -n "$NS" -l app.kubernetes.io/name=nango -o wide
kubectl logs -n "$NS" deploy/nango -c nango --since=30m --prefix
kubectl describe httproute nango-https -n "$NS"
kubectl get cluster.postgresql.cnpg.io/nango-postgres -n "$NS" -o wide
```

Nango must pass `/health` and `/ready` on port `3003`. The backend loads Nango's
provider catalog during startup, so a Nango or Nango PostgreSQL failure may
surface as an API readiness failure. OAuth callbacks enter through
`connections.usehivy.com` or `staging.connections.usehivy.com`; internal API
calls use the namespace-local `nango:3003` Service.

## PostgreSQL and migrations

CloudNativePG owns `backend-postgres`, `nango-postgres`, and, in production,
`microsandbox-postgres`.

```sh
NS=production
kubectl get cluster.postgresql.cnpg.io -n "$NS" -o wide
kubectl describe cluster.postgresql.cnpg.io/backend-postgres -n "$NS"
kubectl get pods -n "$NS" -l cnpg.io/cluster=backend-postgres -o wide
kubectl logs -n "$NS" -l cnpg.io/cluster=backend-postgres \
  -c postgres --since=30m --prefix
kubectl get backup,scheduledbackup -n "$NS"
kubectl get pods -n cnpg-system -o wide
kubectl logs -n cnpg-system deploy/cnpg-controller-manager --since=30m
kubectl logs -n cnpg-system deploy/barman-cloud --since=30m
```

Each API and worker Pod runs `/hivy migrate up` before its main process.
Concurrent starts rely on Goose's PostgreSQL advisory lock. If a rollout stalls,
inspect every new Pod's `migrate` logs and the database cluster state. Don't run
an ad hoc migration until you know whether the init containers are blocked,
failed, or still holding the lock.

Use CloudNativePG's generated read-write Service (`backend-postgres-rw` or
`nango-postgres-rw`) for clients. Don't point applications at a numbered Pod;
the primary ordinal may change after failover.

## Redis

Staging uses a persistent standalone `Redis` resource. Production uses a
`RedisCluster` with three leaders and three followers.

```sh
kubectl get redis.redis.redis.opstreelabs.in/backend-redis -n staging -o yaml
kubectl get statefulset,pod,pvc -n staging -l app.kubernetes.io/name=backend-redis

kubectl get rediscluster.redis.redis.opstreelabs.in/backend-redis \
  -n production -o yaml
kubectl get pods -n production -l redis_setup_type=cluster -o wide
kubectl logs -n production -l redis_setup_type=cluster \
  --since=30m --all-containers --prefix

kubectl get pods -n redis-operator -o wide
kubectl logs -n redis-operator deploy/redis-operator --since=30m
```

Inspect CR status and operator events before deleting a Redis Pod. Production
PVC retention is `Retain` for deletion and scale-down, so a stale PVC may remain
after manual topology changes. The application uses Redis Cluster addresses in
production and a single address in staging; a configuration copied between
environments will fail differently even if port `6379` answers.

## Qdrant

Qdrant runs as a separately rendered StatefulSet named `qdrant`; it isn't part
of the Kustomize render. Staging has one peer with clustering disabled.
Production has three peers and uses ports `6333` (HTTP), `6334` (gRPC), and
`6335` (peer traffic).

```sh
NS=staging
kubectl get statefulset/qdrant svc/qdrant svc/qdrant-headless -n "$NS"
kubectl rollout status statefulset/qdrant -n "$NS" --timeout=5m
kubectl get pods,pvc -n "$NS" -l app.kubernetes.io/name=qdrant -o wide
kubectl logs -n "$NS" statefulset/qdrant --since=30m --prefix
kubectl describe networkpolicy/qdrant-isolation -n "$NS"
kubectl get cronjob/qdrant-backup -n "$NS"
```

The backend reaches Qdrant over gRPC and must present the API key. The backup
job uses authenticated HTTP and writes snapshots directly to Hetzner Object
Storage. A successful Pod health check doesn't prove that the API key, gRPC
policy, collection state, or S3 snapshot path works.

## Microsandbox control plane and runners

The shared control plane runs only in `production`; staging calls its ClusterIP
through `microsandbox-control.production.svc.cluster.local:8080`.

```sh
kubectl get deploy,svc,pod -n production \
  -l app.kubernetes.io/name=microsandbox-control -o wide
kubectl logs -n production deploy/microsandbox-control \
  -c control --since=30m --prefix
kubectl get endpointslice -n production \
  -l kubernetes.io/service-name=microsandbox-control
kubectl get cluster.postgresql.cnpg.io/microsandbox-postgres -n production
```

Runners are systemd services on bare metal, outside Kubernetes:

```sh
cd ansible
ansible runners -m command -a 'systemctl is-active microsandbox-runner'
ansible runners -m shell -a 'journalctl -u microsandbox-runner --since "30 minutes ago" --no-pager'
ansible runners -m command -a 'ss -lntup'
ansible-playbook playbooks/phase4-validate.yml
```

The runner API listens on private port `8081`; preview ports occupy
`30000:60999`. Kubernetes nodes may reach both ranges through UFW. Runner calls
back to the control plane use NodePort `32080`, which is admitted only for the
configured runner IPs on K3s node firewalls.

For a failed sandbox session, trace in this order:

1. API or worker logs for the session UUID and selected runner.
2. Microsandbox control logs for provisioning, heartbeat, or runner selection.
3. The chosen runner's systemd journal.
4. Preview cache and preview proxy logs if provisioning succeeded but the URL
   fails.
5. LLM proxy API logs if the runtime started but model traffic failed.

Don't delete the sandbox, restart runners, or rerun provisioning until these
five sources are captured.

## Gateway, certificates, and public DNS

An `HTTPRoute` must show `Accepted=True` and `ResolvedRefs=True` for its intended
listener. Check both route status and backend endpoints:

```sh
kubectl get gateway -n ingress-public
kubectl describe gateway public -n ingress-public
kubectl get httproute -A
kubectl describe httproute backend-api -n staging
kubectl get svc,endpointslice -n staging
kubectl get certificate,certificaterequest -A
kubectl describe certificate preview-usehivy -n ingress-public
kubectl get challenge,order -A
kubectl logs -n cert-manager deploy/cert-manager --since=30m
```

Test the load balancer without waiting for DNS caches:

```sh
curl --resolve staging.api.usehivy.com:80:65.109.40.68 \
  -I http://staging.api.usehivy.com/
curl --resolve staging.api.usehivy.com:443:65.109.40.68 \
  -I https://staging.api.usehivy.com/healthz
openssl s_client -connect 65.109.40.68:443 \
  -servername staging.api.usehivy.com </dev/null
```

Expected public flow: client to Hetzner LB, TCP with PROXY v2 to node ports
`10080` or `10443`, Cilium Envoy TLS termination, HTTPRoute, Service, Pod. A
healthy certificate with a `404` or `401` usually proves the network and TLS
path but not the requested application route or credentials.

The committed certificate currently contains API, proxy, connections, web,
preview, and monitor names. It does **not** contain
`staging.mcp.usehivy.com`, although staging declares an HTTPRoute for that host;
production declares no MCP HTTPRoute. Treat MCP TLS failures as a manifest gap,
not a cert-manager outage. Don't add an `AAAA` record for the public load
balancer until the documented Cilium IPv6/PROXY issue passes testing.

## Private DNS and runner HAProxy

Runner CoreDNS resolves the API and LLM proxy hostnames to that same runner's
private IP. HAProxy sends TLS with PROXY v2 to healthy K3s ingress nodes:

```sh
cd ansible
ansible-playbook playbooks/runner-coredns.yml
ansible-playbook playbooks/runner-haproxy.yml

ansible runners -m shell -a \
  'getent ahostsv4 api.usehivy.com; getent ahostsv4 proxy.usehivy.com'
ansible runners -m shell -a \
  'printf "show stat\n" | socat - UNIX-CONNECT:/run/haproxy/admin.sock'
ansible runners -m command -a 'systemctl is-active hivy-coredns haproxy'
```

An API or proxy lookup on `runner0` should return `10.80.1.2`; `runner1` should
return `10.80.1.3`. HAProxy statistics should list `k8s-0` and `k8s-1` as `UP`.
The inventory group `k3s_ingress`, not DNS, decides which Kubernetes nodes
receive this traffic.

Cluster-internal preview DNS uses a different path. CoreDNS rewrites the preview
wildcard to the production TLS bridge, then that HAProxy Pod adds PROXY v2:

```sh
kubectl get configmap/coredns-custom -n kube-system -o yaml
kubectl rollout status deploy/coredns -n kube-system --timeout=3m
kubectl get deploy,svc -n production \
  -l app.kubernetes.io/name=microsandbox-preview-tls-bridge
kubectl logs -n production deploy/microsandbox-preview-tls-bridge --since=30m
kubectl logs -n production deploy/microsandbox-preview-proxy --since=30m
```

## Zot registry

Zot runs in production and remains private. Runner `/etc/hosts` resolves
`registry.usehivy.com` to its local HAProxy listener on port `5000`; HAProxy
forwards to NodePort `32500` on healthy K3s nodes. Zot terminates TLS itself.

```sh
kubectl get statefulset,svc,pod,pvc -n production \
  -l app.kubernetes.io/name=zot -o wide
kubectl logs -n production statefulset/zot --since=30m
kubectl get certificate zot-registry -n production

cd ansible
ansible runners -m shell -a \
  'getent hosts registry.usehivy.com; curl -fsS https://registry.usehivy.com:5000/v2/'
```

If a runner can't pull an image, check Zot readiness and certificate first,
then HAProxy's registry backend, K3s host firewall port `32500`, runner host
resolution, and BuildKit registry configuration.

## Longhorn and PVCs

Never delete or recreate a PVC as a diagnostic shortcut. Start with the claim,
its Longhorn volume, replica placement, and node disk state:

```sh
kubectl get storageclass
kubectl get pvc -A
kubectl describe pvc -n staging <claim-name>
kubectl get volumes.longhorn.io -n longhorn-system
kubectl get replicas.longhorn.io -n longhorn-system -o wide
kubectl get nodes.longhorn.io -n longhorn-system -o yaml
kubectl get pods -n longhorn-system -o wide
kubectl get events -n longhorn-system --sort-by=.lastTimestamp | tail -80
```

A volume should report `healthy` with two scheduled replicas on distinct
current nodes. `WaitForFirstConsumer` means a new claim can remain unbound until
its Pod is scheduled. Longhorn reserves 15 percent of each default disk and
stops scheduling before free capacity reaches its configured minimum; raw disk
free space and schedulable Longhorn capacity aren't the same number.

When a node loses storage access, inspect `iscsid`, required kernel modules,
and the masked multipath services on that host before touching a volume:

```sh
systemctl status iscsid
lsmod | grep -E 'iscsi_tcp|dm_crypt|nfs'
systemctl is-enabled multipathd.service multipathd.socket
```

## Nodes, Cilium, and scheduling

```sh
kubectl get nodes -o wide
kubectl describe node k8s-0
kubectl describe node k8s-1
kubectl top nodes
kubectl top pods -A --sort-by=memory
kubectl get pods -A --field-selector=status.phase=Pending
kubectl get events -A --sort-by=.lastTimestamp | tail -100

kubectl rollout status daemonset/cilium -n kube-system --timeout=5m
kubectl rollout status daemonset/cilium-envoy -n kube-system --timeout=5m
kubectl rollout status deployment/cilium-operator -n kube-system --timeout=5m
kubectl logs -n kube-system daemonset/cilium --since=30m --prefix
kubectl logs -n kube-system daemonset/cilium-envoy --since=30m --prefix
```

For a Pending Pod, `kubectl describe pod` gives the scheduler's rejection
reason. Compare requests, not limits, with node allocatable capacity and check
the staging ResourceQuota. Each node reserves resources for the operating
system and Kubernetes, so its schedulable capacity is lower than its physical
RAM and CPU count.

K3s embedded etcd currently has two members. Both must remain available for
quorum, which means the control plane isn't tolerant of either member failing.
Don't drain or reboot one server casually until a third server joins and etcd
health has been checked.

## Backups

Start with controller status and the most recent job instead of inspecting S3
credentials:

```sh
NS=production
kubectl get scheduledbackup,backup -n "$NS"
kubectl get cronjob,job -n "$NS"
kubectl logs -n "$NS" job/<job-name> --all-containers
kubectl logs -n cnpg-system deploy/barman-cloud --since=1h
```

PostgreSQL success requires the base backup and ending WAL. Redis jobs export,
load-test, upload, and verify RDB objects. Qdrant asks each configured peer to
create an S3-backed collection snapshot. A green CronJob status proves the job
ran; only a restore into a temporary database or collection proves recovery.

## Safe recovery rules

- Capture logs, events, object YAML, and timestamps before a restart.
- Don't edit generated operator objects when a committed source manifest or
  Helm values file owns them.
- Don't delete a database Pod and its PVC together.
- Don't print Secrets to find a typo; compare key names and checksums instead.
- Apply to staging first unless the incident itself blocks staging validation.
- After a fix, repeat the exact failing request and verify both application
  behavior and the expected Grafana evidence.
