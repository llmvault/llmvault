---
name: platform-status-check
description: Read-only cluster and application health check via Grafana. Query live metrics across cluster, services, API SLOs, LLM, queues, sandboxes, data services, observability pipeline, and backups. Compile a structured status report and optionally email it. Use this skill for rapid health snapshots, SLO reviews, on-call handoffs, or daily status reports without requiring Kubernetes access.
---

# Platform Status Check

Query the Hivy Grafana server to compile a complete, point-in-time health report for the production (or staging) environment. No Kubernetes access is required — all evidence comes from VictoriaMetrics and VictoriaLogs via the Grafana MCP tools.

## Scope and boundaries

- **Read-only.** Query data sources only. Never create, update, delete, or modify any Grafana resource, dashboard, alert, or data source.
- **Point-in-time.** All queries are instant snapshots. State the observation window in the report.
- **No secrets.** Never print data source credentials, connection strings, or any value returned by the Grafana API that is not a metric result.
- **No speculation.** Report what the metrics show. Distinguish confirmed issues from risks and monitoring gaps.
- **Complementary to `platform-engineering-agent`.** That skill uses kubectl for deep cluster investigation. This skill uses Grafana for a fast, broad health snapshot. Use both when diagnosing incidents — this one first for the overview, then `platform-engineering-agent` for drill-down.

## Prerequisites

Load the Grafana MCP tools at the start of every run:

```
load_tools: ["connection-grafana_list_data_sources", "connection-grafana_query_data_source", "connection-grafana_search_dashboards", "connection-grafana_get_dashboard"]
```

Verify data sources are reachable by calling `connection-grafana_list_data_sources`. The following must be present:

| Data source | UID | Type |
|---|---|---|
| VictoriaMetrics | `VictoriaMetrics` | prometheus |
| VictoriaLogs | `VictoriaLogs` | victoriametrics-logs-datasource |

If either is missing, stop and report the gap.

## Query conventions

- All metric queries target the `VictoriaMetrics` data source (UID `VictoriaMetrics`, type `prometheus`).
- Use `namespace="$environment"` in every query, where `$environment` is `production` (default) or `staging` (if requested).
- All queries are `instant: true` — point-in-time snapshots, not range series.
- Batch independent queries into a single `connection-grafana_query_data_source` call (up to ~8 queries per batch) for efficiency.
- Use `from: "now-1h"` and `to: "now"` as the query time range.
- When a query returns an empty frame, interpret it as "no data" (metric not present or no matching series) and report it as such — do not assume a value of zero for rate ratios.

## Investigation workflow

Work through the sections below in order. Each section lists what to check, the exact PromQL expression, and how to interpret the result.

### 1. Cluster health

Query these in one batch:

| Check | PromQL | Interpretation |
|---|---|---|
| Nodes ready | `sum(kube_node_status_condition{condition="Ready",status="true"})` | Should match total node count |
| Nodes not ready | `sum(kube_node_status_condition{condition="Ready",status!="true"}) or vector(0)` | Must be 0 |
| Pods running | `sum(kube_pod_status_phase{namespace="$env",phase="Running"})` | Baseline count |
| Pods not ready | `sum(kube_pod_status_ready{namespace="$env",condition="false"} * on(namespace,pod) group_left() kube_pod_status_phase{namespace="$env",phase="Running"})` | Must be 0 |
| Container restarts (1h) | `sum(increase(kube_pod_container_status_restarts_total{namespace="$env"}[1h]))` | 0 = clean; >0 investigate |
| Unavailable replicas | `sum(kube_deployment_status_replicas_unavailable{namespace="$env"})` | Must be 0 |
| Rollouts in progress | `count((kube_deployment_metadata_generation{namespace="$env"} != on(namespace,deployment) kube_deployment_status_observed_generation{namespace="$env"}) == 1)` | 0 = stable; >0 = deploying |
| PVC max usage | `max(kubelet_volume_stats_used_bytes{namespace="$env"} / clamp_min(kubelet_volume_stats_capacity_bytes{namespace="$env"}, 1))` | <70% green; 70-85% yellow; >85% red |

Query resource usage in a second batch:

| Check | PromQL | Interpretation |
|---|---|---|
| CPU usage | `sum(rate(container_cpu_usage_seconds_total{namespace="$env",container!="",image!=""}[5m]))` | Cores in use |
| Memory usage | `sum(container_memory_working_set_bytes{namespace="$env",container!="",image!=""})` | Bytes in use |
| Storage used | `sum(kubelet_volume_stats_used_bytes{namespace="$env"})` | Bytes in use |

### 2. Service readiness

Query all 9 services in one batch. Each uses the same pattern:

```
min(kube_pod_status_ready{namespace="$env",condition="true",pod=~"PATTERN"}) or vector(0)
```

| Service | Pod pattern | refId |
|---|---|---|
| API | `backend-api-.*` | `api_ready` |
| Web | `web-.*` | `web_ready` |
| Asynq Worker | `backend-worker-.*` | `worker_ready` |
| PostgreSQL | `.*-postgres-[0-9]+` | `pg_ready` |
| Redis | `(backend-redis-(leader\|follower)-[0-9]+\|backend-redis-[0-9]+)` | `redis_ready` |
| Qdrant | `qdrant-[0-9]+` | `qdrant_ready` |
| Microsandbox | `microsandbox-(control\|preview-(cache\|proxy\|redis))-.*` | `msb_ready` |
| Nango | `nango-[a-f0-9]{8,10}-.*` | `nango_ready` |
| Zot Registry | `zot-[0-9]+` | `zot_ready` |

Interpretation: `1` = all pods ready (healthy), `0` = at least one pod not ready (down). Report each service as 🟢 or 🔴.

### 3. API and SLO health

Query in one batch:

| Check | PromQL | Interpretation |
|---|---|---|
| Requests/sec | `sum(rate(hivy_http_requests_total{namespace="$env"}[5m]))` | Current load |
| 5xx error rate | `sum(rate(hivy_http_requests_total{namespace="$env",status_class="5xx"}[5m])) / clamp_min(sum(rate(hivy_http_requests_total{namespace="$env"}[5m])), 0.001)` | <1% green; 1-5% yellow; >5% red |
| p95 latency | `histogram_quantile(0.95, sum by (le) (rate(hivy_http_request_duration_seconds_bucket{namespace="$env"}[5m])))` | <100ms green; 100-500ms yellow; >500ms red |
| In-flight requests | `sum(hivy_http_requests_in_flight{namespace="$env"})` | Current concurrency |
| Session-create p95 | `histogram_quantile(0.95, sum by (le) (rate(hivy_http_request_duration_seconds_bucket{namespace="$env",route=~".*/sessions.*",method="POST"}[10m])))` | <500ms green; 500ms-2s yellow; >2s red |
| Public probes up | `sum(probe_success{job="hivy-public-journeys"})` | All should be 1 |
| Public probe duration | `max(probe_duration_seconds{job="hivy-public-journeys"})` | <1s green; 1-3s yellow; >3s red |

If the 5xx error rate query returns an empty frame, it means there were zero 5xx responses in the window — report as 0% (green).

### 4. LLM and agent health

Query in one batch:

| Check | PromQL | Interpretation |
|---|---|---|
| Generations/sec | `sum(rate(hivy_llm_generations_total{namespace="$env"}[5m]))` | Current LLM load |
| Generation error rate | `sum(rate(hivy_llm_generations_total{namespace="$env",status="error"}[5m])) / clamp_min(sum(rate(hivy_llm_generations_total{namespace="$env"}[5m])), 0.001)` | <1% green; 1-5% yellow; >5% red |
| p95 generation duration | `histogram_quantile(0.95, sum by (le) (rate(hivy_llm_generation_duration_seconds_bucket{namespace="$env"}[10m])))` | Context-dependent; flag >120s |
| LLM cost (1h) | `sum(increase(hivy_llm_cost_usd_total{namespace="$env"}[1h]))` | Track for anomalies |

If the generation error rate query returns an empty frame, report as 0% (green) — no errors in the window.

### 5. Async work and queues

Query in one batch:

| Check | PromQL | Interpretation |
|---|---|---|
| Pending tasks | `sum(max by (queue) (hivy_asynq_queue_tasks{namespace="$env",state="pending"}))` | 0 = clean; >0 = backlog |
| Retry queue | `sum(max by (queue) (hivy_asynq_queue_tasks{namespace="$env",state="retry"}))` | 0 = clean; >0 = tasks failing |
| Archived tasks | `sum(max by (queue) (hivy_asynq_queue_tasks{namespace="$env",state="archived"}))` | Accumulated; flag if growing |
| Oldest pending age | `max(hivy_asynq_queue_latency_seconds{namespace="$env"})` | 0 = no backlog; >60s = stale |

### 6. Sandbox and runner health

Query in one batch:

| Check | PromQL | Interpretation |
|---|---|---|
| Healthy runners | `sum(hivy_microsandbox_runner_status{namespace="$env",status="healthy"})` | Should be >0 |
| Unhealthy runners | `sum(hivy_microsandbox_runner_status{namespace="$env",status="unhealthy"})` | Must be 0 |
| Running sandboxes | `sum(hivy_microsandbox_sandboxes{namespace="$env",status="running"})` | Current active count |
| Max heartbeat age | `max(hivy_microsandbox_runner_heartbeat_age_seconds{namespace="$env"})` | <30s green; >30s yellow; >60s red |

### 7. Data services

Query in one batch:

| Check | PromQL | Interpretation |
|---|---|---|
| PG collectors up | `sum(cnpg_collector_up{namespace="$env"})` | Should be >=1 |
| Redis up | `max(redis_up{namespace="$env"})` | 1 = up |
| Qdrant ready pods | `sum(kube_pod_status_ready{namespace="$env",pod=~"qdrant-.*",condition="true"})` | Should be >=1 |
| PG replication lag | `max(cnpg_pg_replication_lag{namespace="$env"})` | 0s green; >5s yellow; >30s red |

### 8. Observability pipeline

Query in one batch:

| Check | PromQL | Interpretation |
|---|---|---|
| Failed scrapes | `count(up == 0)` | 0 = clean; >0 = monitoring gap |
| Metrics ingestion rate | `sum(rate(vm_rows_inserted_total[5m]))` | Baseline throughput |
| Active firing alerts | `sum(ALERTS{alertstate="firing"})` | 0 = clean; >0 = triage needed |
| Remote-write backlog | `sum by (pod) (vmagent_remotewrite_pending_data_bytes)` | 0 = clean; growing = ingestion lag |

### 9. Backups and recovery

Query in one batch:

| Check | PromQL | Interpretation |
|---|---|---|
| Failed backup jobs (24h) | `sum(increase(kube_job_status_failed{namespace="$env",job_name=~".*backup.*"}[24h]))` | 0 = clean; >0 = investigate |
| Suspended backup CronJobs | `sum(kube_cronjob_spec_suspend{namespace="$env",cronjob=~".*backup.*"})` | 0 = running; >0 = paused |

## Compile the report

Write the report as clean GitHub-flavored Markdown. Use this structure:

### Report structure

```markdown
# Hivy Cluster & Application Health Report

**Generated:** <UTC date/time>
**Environment:** production (or staging)

---

## Overall Status: <emoji> <HEALTHY | DEGRADED | CRITICAL>

<1-2 sentence summary>

---

## 1. Cluster Health
<table with all cluster metrics, status emojis>

## 2. Service Readiness
<table with all 9 services, status emojis>

## 3. API & SLO Health
<table with API metrics, status emojis>

## 4. LLM / Agent Health
<table with LLM metrics, status emojis>

## 5. Async Work & Queues
<table with queue metrics, status emojis>

## 6. Sandbox & Runner Health
<table with sandbox metrics, status emojis>

## 7. Data Services
<table with data service metrics, status emojis>

## 8. Observability Pipeline
<table with observability metrics, status emojis>

## 9. Backups & Recovery
<table with backup metrics, status emojis>

---

## Items Requiring Attention
<numbered list of anything yellow or red, with recommended action>

---

## Dashboards for Drill-Down
<links to relevant Grafana dashboards>
```

### Status indicators

Use these thresholds to assign status emojis:

| Indicator | 🟢 Green | 🟡 Yellow | 🔴 Red |
|---|---|---|---|
| Service readiness | = 1 | n/a | = 0 |
| Pods not ready | = 0 | n/a | > 0 |
| Container restarts (1h) | = 0 | 1-4 | ≥ 5 |
| 5xx error rate | < 1% | 1-5% | > 5% |
| API p95 latency | < 100ms | 100-500ms | > 500ms |
| PVC max usage | < 70% | 70-85% | > 85% |
| PG replication lag | 0s | > 5s | > 30s |
| Runner heartbeat age | < 30s | 30-60s | > 60s |
| Failed scrapes | 0 | n/a | > 0 |
| Firing alerts | 0 | 1-10 | > 10 |
| Failed backup jobs (24h) | 0 | n/a | > 0 |
| Archived tasks | < 1000 | 1000-5000 | > 5000 |

### Overall status

- **🟢 HEALTHY:** All services ready, no red items, zero or only informational yellow items.
- **🟡 DEGRADED:** All critical services ready, but one or more yellow items need attention.
- **🔴 CRITICAL:** One or more services down, or any red item that affects user-facing functionality.

### Drill-down dashboard links

Include these links in every report:

- Environment Overview: `/d/hivy-overview/hivy-environment-overview`
- API Reliability: `/d/hivy-api-reliability/hivy-api-reliability-and-slos`
- Agent Runs and LLM: `/d/hivy-agent-llm/hivy-agent-runs-and-llm`
- Async Work and Queues: `/d/hivy-asynq/hivy-async-work-and-queues`
- Sandbox Capacity: `/d/hivy-sandbox-capacity/hivy-sandbox-capacity-and-lifecycle`
- Data Services: `/d/hivy-data-services/hivy-data-services`
- Observability Pipeline: `/d/hivy-observability-pipeline/hivy-observability-pipeline`
- Backups: `/d/hivy-backups/hivy-backups-and-recovery-readiness`
- Alertmanager: `/d/alertmanager-overview/alertmanager-overview`

## Deliver

Write the report to a file in the workspace:

```bash
REPORT_PATH="/workspace/health-report.md"
```

If the task explicitly requests email delivery:

1. Load the `hivy_send_email` MCP tool.
2. Call it with `markdown_file_path` set to the report path and the recipient address provided by the user.
3. Do not read the report contents into chat or pass them as a tool argument — the email service reads the file directly.

If email was not requested, return the report path and a concise chat summary.

### Chat summary

Always end with a concise chat summary containing:

- Overall status (emoji + word)
- TL;DR in one sentence
- Numbered list of items requiring attention (if any)
- Confirmation of email delivery (if applicable) or report path

## Limitations

- This skill queries Grafana metrics only. It cannot inspect Kubernetes objects, run kubectl, read pod logs directly, or check object-storage backup contents. Use `platform-engineering-agent` for deep investigation.
- Metrics are point-in-time snapshots. Transient issues between scrape intervals may not appear.
- Empty query results mean "no data for this metric in the window" — not necessarily "zero." Interpret carefully for ratio queries.
- Firing alerts may include expected or silenced alerts. Recommend reviewing Alertmanager for triage.
- This skill does not verify backup restore viability — only whether Kubernetes reports backup job success or failure.
