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
| VMAgent | Scrapes cluster, application, PostgreSQL, Redis, Qdrant, and synthetic metrics | Sends to VictoriaMetrics |
| VLAgent | Runs on every node and tails every container log | Flushes every second; buffers up to 1 GB per node under `/var/lib/vlagent` during an outage |
| kube-state-metrics | Reports object state such as Ready Pods and Deployment replicas | Scraped by VMAgent |
| node exporter and kubelet/cAdvisor | Report node, Pod, container, network, and volume use | Scraped by VMAgent |
| VMAlert and Alertmanager | Evaluate the chart's default rules every 20 seconds | Alerts appear in the UI; no external receiver is configured |
| Grafana | Reads both stores and serves dashboards | Configuration on a 2 GiB Longhorn PVC |
| Redis exporter | Reads content-free Redis `INFO` and latency data | Scraped every 30 seconds |
| Blackbox exporter | Probes public API and web health endpoints | Scraped every 30 seconds |

Bare-metal runners use the same stores without joining Kubernetes. Ansible
installs a node exporter, VMAgent, VLAgent, and systemd journal uploader on
every runner. The agents bind only to loopback, buffer on the runner's local
disk, and write over the private network to `telemetry.usehivy.com`. A
two-replica VMAuth service checks a write-only bearer token before routing
metrics to VictoriaMetrics and journals to VictoriaLogs.

Every sandbox image runs `systemd-journal-upload`. The runner injects a
sandbox-specific, HMAC-signed upload URL when it creates the sandbox and accepts
that URL only while the sandbox still exists. The receiver replaces all
caller-supplied identity headers with labels obtained from the runner backend,
then sends the journal through the runner-local VLAgent. This gives sandbox
records trusted `runner_id`, `sandbox_id`, `session_id`,
`provisioning_attempt_id`, `trace_id`, `org_id`, `agent_id`, `harness`, and
`source=sandbox` fields without exposing the central telemetry credential to a
sandbox.

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

Use **Hivy / Runners and Sandboxes** for the bare-metal fleet. It includes runner
availability, filesystem and memory pressure, telemetry queue depth, service
state, error volume by source and runner, and a searchable error log table.
The dashboard variables narrow the view to an environment, runner, sandbox, org,
agent, harness, or service. Runner system journals use `source=runner`; sandbox
journals use `source=sandbox`.

Use **Hivy / Session Forensics** when a session UUID is known. Paste the exact
UUID into **Session ID**. The dashboard joins structured fields and legacy raw
messages across API, worker, microsandbox control, runner, and runtime streams.
It shows total provisioning duration, every successful provisioning phase,
warnings and errors, sanitized tool-call lifecycle records and durations, and
the complete chronological session timeline. `provisioning_attempt_id`
distinguishes retries for the same session; `trace_id` links the provisioning
request across HTTP service boundaries.

The application dashboard suite is generated by
`scripts/observability/generate-dashboards.mjs`. Run that script after editing
its dashboard definitions; the generated JSON files are the artifacts loaded by
Grafana. Then run `scripts/observability/validate-dashboards.mjs`; it checks
dashboard identity, panel layout, and that SQL panels do not query customer
content or secret columns.

| Dashboard | Primary question |
| --- | --- |
| Customer Support 360 | What is the current operational state of this org or session? |
| API Reliability and SLOs | Which route or service is slow or returning errors? |
| Agent Runs and LLM | Is a provider, model, token class, or provisioning phase unhealthy or expensive? |
| Async Work and Queues | Are tasks delayed, retrying, archived, or taking too long? |
| Sandbox Capacity and Lifecycle | Is runner capacity, pressure, heartbeat health, or lifecycle state blocking provisioning? |
| Automations and Integrations | Are schedules or trigger deliveries failing or delayed? |
| Knowledge and RAG | Which source is stale, repeatedly failing, or accumulating unresolved index errors? |
| Data Services | Are PostgreSQL, Redis, Qdrant, or their volumes unhealthy? |
| Billing and Security | Is usage unbilled, are purchases failing, or did access activity spike? |
| Apps, Sheets, and Email | Which product operation is stuck or failing? |
| Deployments and Change Correlation | Did replica availability or restarts change around a rollout? |
| Backups and Recovery Readiness | Are backup jobs failing or storage volumes filling? |
| Observability Pipeline | Is telemetry itself missing, delayed, or failing to ingest? |
| Product Journey SLOs | Are public endpoints, session creation, provisioning, or delivery journeys degraded? |

## Content-safe support data

Grafana has one read-only PostgreSQL datasource per application environment.
The `hivy_observability` login can select only the nine
`observability_*` views created by Goose migration 9. Those views expose
operational IDs, timestamps, statuses, durations, counts, token totals, cost
totals, and error flags. They do not expose prompts, session messages, email
addresses or bodies, webhook payloads, integration configuration, credentials,
API keys, or raw error text.

The datasource selector is tied to the dashboard's environment selector.
Grafana keeps at most five production and three staging database connections,
so support queries cannot consume the application connection pool. Dashboard
queries also have explicit row limits.

Do not add a base application table to the Grafana role. Add or extend a
sanitized view in a migration, grant that view to `hivy_observability`, and
update the generated dashboard query. Customer IDs remain query variables and
log fields; they are intentionally not Prometheus labels.

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

Container resource and readiness metrics exist for all workloads. VM-native
scrape objects also collect the Hivy API, MCP, worker, Microsandbox control,
CloudNativePG, Qdrant, and Redis exporter endpoints. The Prometheus
`PodMonitor`/`ServiceMonitor` switches remain disabled because this stack
deliberately disables the VictoriaMetrics operator's Prometheus-object
converter; `application-scrapes.yaml` is the single source of workload scrape
configuration.

Application metrics use bounded labels only. HTTP metrics use normalized Chi
route patterns rather than request paths. LLM metrics use provider and model,
not org or session IDs. Asynq collectors expose queue counts and latency but
never task payloads. Per-customer diagnosis stays in the sanitized SQL views and
VictoriaLogs.

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

Generate the ignored credentials first. This command preserves existing values
unless `--refresh` is passed and keeps the environment database-role passwords
in sync with Grafana's datasource secret. It also mirrors the existing staging
and production Qdrant API keys into the observability-only metrics secret:

```sh
kubernetes/observability/generate-secrets.sh
```

On the first rollout, apply each environment's data phase and wait for
CloudNativePG before the workload phase. The data phase creates the managed
`hivy_observability` role; the API/worker migration init container then creates
and grants the support views. This is the same phased order documented in
`deployments-and-ci.md`.

Install or upgrade the monitoring stack, then apply its Kustomize resources:

```sh
kubectl apply -f kubernetes/observability/namespace.yaml
kubectl apply -k kubernetes/config/env/observability -n observability

helm upgrade --install obs vm/victoria-metrics-k8s-stack \
  --version 0.86.2 \
  --namespace observability \
  --values kubernetes/observability/values.yaml \
  --wait --timeout 15m

kubectl apply -k kubernetes/observability
kubectl apply -f kubernetes/ingress/public/certificate.yaml
```

The dashboard sidecar watches ConfigMaps labeled `grafana_dashboard=1`, so an
updated checked-in dashboard normally appears without restarting Grafana.
After changing retention or storage requests, inspect the relevant PVC and
custom resource before assuming Helm resized it.

To rotate it, replace `token=` in
`kubernetes/config/env/observability/telemetry-ingest.env`, apply the Kustomize
directory, and rerun `ansible-playbook playbooks/runner-observability.yml` from
`ansible/`. The runner agents restart only when their credential file changes.

## Scaling nodes and runners

New Kubernetes nodes need no dashboard or scrape-list edits. The VMAgent
Kubernetes discovery configuration automatically picks up kubelet,
node-exporter, workload, and database targets after the normal K3s Ansible
install.

New runners receive node exporter, VMAgent, VLAgent, runner API scraping, and
runner log-ingest scraping from the existing `runner-observability` role.
Adding the runner to the `runners` inventory group and running the documented
runner phases is sufficient for telemetry. The central metric store identifies
it through the Ansible `runner_name` external label; the control collector adds
its capacity and sandbox state after registration.
