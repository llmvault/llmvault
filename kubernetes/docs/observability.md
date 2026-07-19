# Observability

Grafana at `https://monitor.usehivy.com` is the entry point for cluster logs,
Pod and node metrics, dashboards, and active alerts. Anonymous access and
self-service sign-up are disabled. The administrator credential comes from the
ignored file `kubernetes/config/env/observability/grafana-admin.env`.

## Components and retention

The official `victoria-metrics-k8s-stack` chart is pinned to `0.86.2` and runs
inside the `observability` namespace.

| Component | Job | Retention or buffer |
| --- | --- | --- |
| VictoriaMetrics | Stores Prometheus-compatible metrics | 30 days on a 15 GiB Longhorn PVC |
| VictoriaLogs | Stores Kubernetes container logs | 14 days on a 30 GiB Longhorn PVC |
| VMAgent | Scrapes cluster and workload metrics every 15 seconds | Sends to VictoriaMetrics |
| VLAgent | Runs on every node and tails every container log | Flushes every second; buffers up to 1 GB per node under `/var/lib/vlagent` during an outage |
| kube-state-metrics | Reports object state such as Ready Pods and Deployment replicas | Scraped by VMAgent |
| node exporter and kubelet/cAdvisor | Report node, Pod, container, network, and volume use | Scraped by VMAgent |
| VMAlert and Alertmanager | Evaluate the chart's default rules every 20 seconds | Alerts appear in the UI; no external receiver is configured |
| Grafana | Reads both stores and serves dashboards | Configuration on a 2 GiB Longhorn PVC |

K3s embeds the controller manager, scheduler, etcd, and kube-proxy replacement,
so the chart's scrapers for those standalone components are disabled. Cilium
replaces kube-proxy. API server and CoreDNS scraping remain enabled.

VLAgent adds Kubernetes namespace, Pod, container, and node labels. It drops
fields named `authorization`, `cookie`, `password`, `token`, `access_token`, and
`refresh_token`, including the listed capitalization variants, before sending a
record. That filter reduces accidental credential capture, but applications
must still avoid logging secrets under other field names or inside message
text.

## Start with the Hivy dashboards

Grafana opens on **Hivy / Environment Overview**. Set the time range first,
then choose `production` or `staging` from the Environment selector.

The first row has health cards for API, Web, Asynq, Nango, PostgreSQL, Redis,
Qdrant, Microsandbox, and Zot. A card shows `HEALTHY` only when every matching
Pod reports Ready. Click a card to open **Hivy / Service Details** with its Pod
pattern already selected. A service absent from an environment shows `NOT
DEPLOYED`; for example, production-only infrastructure shouldn't be mistaken
for a staging outage.

The overview also shows running and unready Pod counts, restarts during the
last hour, aggregate CPU and memory, persistent storage use, CPU and memory by
Pod, and the newest 500 log lines in that namespace.

On **Hivy / Service Details**:

1. Pick the environment and service.
2. Keep Pod set to `All` when comparing replicas, or select one Pod after a
   restart or uneven resource spike.
3. Set **Search regex** to a session UUID, request ID, error fragment, user ID,
   or `(?i)(error|fatal|panic)`.
4. Read the restart, CPU, memory, and network graphs at the same timestamp as
   the failure; the bottom log panel shows up to 1,000 matching lines.

The storage graph is namespace-wide, not service-specific. Stateless API,
worker, and web Pods don't own a PVC, while PostgreSQL, Redis, Qdrant, Grafana,
VictoriaMetrics, and VictoriaLogs do.

## Find a failed request or session

For a known session UUID, choose the failing environment, open the API service,
and paste the UUID into **Search regex**. Change Service to Asynq if the API
accepted the request but background execution failed. Microsandbox logs cover
the control plane, preview cache, preview proxy, and preview Redis under one
selector. The preview TLS bridge doesn't match that dashboard selector; use
Explore or `kubectl logs` when diagnosing the bridge itself.

If a value doesn't appear in `_msg`, open **Explore**, select `VictoriaLogs`,
and query a structured field directly. These are useful starting queries:

```text
kubernetes.pod_namespace:="staging" AND _msg:~"2632d7e3-bb21-4ffa-b0c3-f4f0fa42c44c" | fields _time, level, _msg, kubernetes.pod_name, kubernetes.container_name | limit 1000

kubernetes.pod_namespace:="production" AND kubernetes.pod_name:~"backend-(api|worker)-.*" AND (level:~"(?i)(error|fatal|panic)" OR _msg:~"(?i)(error|fatal|panic)") | fields _time, level, _msg, kubernetes.pod_name | limit 1000

kubernetes.pod_namespace:="production" AND kubernetes.pod_name:~"microsandbox-.*" | fields _time, level, _msg, kubernetes.pod_name, kubernetes.container_name | limit 1000
```

Widen the time range before assuming the log is missing. Logs use the container
timestamp, and a new Pod name appears after every rollout. Search the whole
namespace when the request crosses API, worker, Nango, or Microsandbox.

## Metrics in Explore

Select `VictoriaMetrics` in Explore for ad hoc PromQL. The dashboard variables
are convenient, but direct queries answer several common questions faster:

```promql
# CPU cores used by each Pod in production
sum by (pod) (rate(container_cpu_usage_seconds_total{namespace="production",container!="",image!=""}[5m]))

# Working-set memory by Pod
sum by (pod) (container_memory_working_set_bytes{namespace="production",container!="",image!=""})

# Restarts in the last hour
sum by (pod,container) (increase(kube_pod_container_status_restarts_total{namespace="production"}[1h]))

# Desired versus available replicas for application Deployments
kube_deployment_spec_replicas{namespace=~"production|staging"}
kube_deployment_status_replicas_available{namespace=~"production|staging"}

# Longhorn-backed PVC bytes used and capacity
kubelet_volume_stats_used_bytes{namespace=~"production|staging|observability"}
kubelet_volume_stats_capacity_bytes{namespace=~"production|staging|observability"}

# Node memory available and filesystem free space
node_memory_MemAvailable_bytes
node_filesystem_avail_bytes{fstype!~"tmpfs|overlay"}
```

Container resource and readiness metrics exist for all workloads. Service-native
database telemetry isn't fully enabled: both CloudNativePG manifests set
`enablePodMonitor: false`, and both Qdrant values files disable ServiceMonitor.
The dashboards can show their Pod health, logs, restarts, CPU, memory, network,
and PVC use, but not PostgreSQL query latency or Qdrant collection statistics.
Redis operator metrics don't replace per-instance Redis command and memory
metrics. Treat this as a known monitoring gap.

## Node and control-plane views

Use Grafana's bundled Kubernetes and node-exporter dashboards for node load,
CPU saturation, filesystem capacity, kubelet health, and Pod scheduling. In
Explore, group container metrics by `node`, or query `kube_node_status_condition`
to distinguish an unhealthy node from a single failed workload.

The cluster currently has two nodes that serve both control-plane and worker
roles. K3s reserves 1 CPU and 2 GiB of memory for the operating system plus the
same amounts for Kubernetes on each node. Capacity panels that use raw node
memory won't subtract those reservations; Kubernetes scheduling data does.

## Alerts

VMAlert evaluates the chart's default Kubernetes rules every 20 seconds and
sends alerts to Alertmanager. CPU throttling must persist for 15 minutes before
that specific rule fires. Alertmanager groups by alert name and namespace, waits
30 seconds before the first group, and repeats an unchanged notification after
12 hours.

The only receiver is named `dashboard-only`. No Slack, email, PagerDuty, or
other outbound destination exists. Check Grafana's alerting pages during an
incident; silence outside Grafana does not mean the system is healthy.

## Check the monitoring pipeline itself

When Grafana lacks fresh data, separate collection, storage, and presentation:

```sh
kubectl get pods -n observability -o wide
kubectl get pvc -n observability
kubectl get httproute -n observability
kubectl describe httproute grafana -n observability
kubectl logs -n observability -l app.kubernetes.io/name=vlagent --since=15m --all-containers
kubectl logs -n observability -l app.kubernetes.io/name=vmagent --since=15m --all-containers
kubectl logs -n observability -l app.kubernetes.io/name=grafana --since=15m --all-containers
```

Chart-generated labels can change between versions. If a selector returns no
Pods, run `kubectl get pods -n observability --show-labels` and use the labels
present on the installed release.

Check VLAgent on every node. One missing DaemonSet Pod creates a node-sized log
blind spot even while Grafana and VictoriaLogs remain healthy. If logs stop
during a VictoriaLogs outage, inspect `/var/lib/vlagent` disk use on the affected
node; the buffer caps each remote-write URL at 1 GB.

## Applying observability changes

Secrets and custom dashboards use Kustomize; the stack itself uses the pinned
Helm release:

```sh
kubectl apply -k kubernetes/observability

helm upgrade --install obs vm/victoria-metrics-k8s-stack \
  --version 0.86.2 \
  --namespace observability \
  --values kubernetes/observability/values.yaml \
  --wait --timeout 15m

kubectl apply -f kubernetes/ingress/public/certificate.yaml
```

The dashboard sidecar watches ConfigMaps labeled `grafana_dashboard=1`, so an
updated checked-in dashboard normally appears without restarting Grafana.
After changing retention or storage requests, inspect the relevant PVC and
custom resource before assuming Helm resized it.
