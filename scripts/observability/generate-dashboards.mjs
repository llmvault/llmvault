#!/usr/bin/env node

import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const outputDir = resolve(root, "kubernetes/observability/dashboards");
mkdirSync(outputDir, { recursive: true });

const metrics = { type: "prometheus", uid: "VictoriaMetrics" };
const logs = { type: "victoriametrics-logs-datasource", uid: "VictoriaLogs" };
const postgres = { type: "grafana-postgresql-datasource", uid: "$support_datasource" };
let nextPanelID = 1;
let nextY = 0;
let nextX = 0;
let rowHeight = 0;

function grid(width = 12, height = 8) {
  if (nextX + width > 24) {
    nextY += rowHeight;
    nextX = 0;
    rowHeight = 0;
  }
  const result = { h: height, w: width, x: nextX, y: nextY };
  nextX += width;
  rowHeight = Math.max(rowHeight, height);
  if (nextX === 24) {
    nextY += rowHeight;
    nextX = 0;
    rowHeight = 0;
  }
  return result;
}

function stat(title, expr, options = {}) {
  const panel = {
    id: nextPanelID++, title, type: "stat", datasource: options.datasource ?? metrics,
    gridPos: grid(6, 5),
    fieldConfig: {
      defaults: {
        color: { mode: "thresholds" },
        mappings: [],
        thresholds: { mode: "absolute", steps: options.thresholds ?? [{ color: "green", value: null }] },
        unit: options.unit ?? "short",
      },
      overrides: [],
    },
    options: {
      colorMode: "value", graphMode: "area", justifyMode: "auto",
      orientation: "auto", reduceOptions: { calcs: ["lastNotNull"], fields: "", values: false },
      textMode: "auto", wideLayout: true,
    },
    targets: [{
      datasource: options.datasource ?? metrics,
      expr,
      format: "time_series",
      instant: true,
      legendFormat: options.legend ?? "",
      refId: "A",
    }],
  };
  return panel;
}

function timeseries(title, expr, options = {}) {
  return {
    id: nextPanelID++, title, type: "timeseries", datasource: options.datasource ?? metrics,
    gridPos: grid(12, options.height ?? 8),
    fieldConfig: {
      defaults: {
        color: { mode: "palette-classic" },
        custom: {
          axisCenteredZero: false, axisColorMode: "text", axisLabel: "",
          axisPlacement: "auto", drawStyle: "line", fillOpacity: 12,
          gradientMode: "none", hideFrom: { legend: false, tooltip: false, viz: false },
          lineInterpolation: "linear", lineWidth: 1, pointSize: 5,
          scaleDistribution: { type: "linear" }, showPoints: "never",
          spanNulls: false, stacking: { group: "A", mode: options.stack ? "normal" : "none" },
          thresholdsStyle: { mode: "off" },
        },
        unit: options.unit ?? "short",
      },
      overrides: [],
    },
    options: {
      legend: { calcs: ["lastNotNull"], displayMode: "table", placement: "bottom", showLegend: true },
      tooltip: { mode: "multi", sort: "desc" },
    },
    targets: [{
      datasource: options.datasource ?? metrics,
      expr,
      legendFormat: options.legend ?? "{{service}}",
      refId: "A",
    }],
  };
}

function sqlStat(title, rawSql, options = {}) {
  const panel = stat(title, "", { ...options, datasource: postgres });
  panel.targets = [{ datasource: postgres, format: "table", rawQuery: true, rawSql, refId: "A" }];
  return panel;
}

function sqlTable(title, rawSql, options = {}) {
  return {
    id: nextPanelID++, title, type: "table", datasource: postgres,
    gridPos: grid(options.width ?? 24, options.height ?? 9),
    fieldConfig: { defaults: { custom: { align: "auto", cellOptions: { type: "auto" }, inspect: false } }, overrides: [] },
    options: { cellHeight: "sm", footer: { countRows: false, fields: "", reducer: ["sum"], show: false }, showHeader: true },
    targets: [{ datasource: postgres, format: "table", rawQuery: true, rawSql, refId: "A" }],
  };
}

function logPanel(title, expr, options = {}) {
  return {
    id: nextPanelID++, title, type: "logs", datasource: logs,
    gridPos: grid(24, options.height ?? 11),
    options: {
      dedupStrategy: "none", enableLogDetails: true, prettifyLogMessage: false,
      showCommonLabels: false, showLabels: false, showTime: true, sortOrder: "Descending", wrapLogMessage: true,
    },
    targets: [{
      datasource: logs, direction: "desc", editorMode: "code", expr,
      maxLines: options.limit ?? 500, queryType: "instant", refId: "A",
    }],
  };
}

function row(title) {
  if (nextX !== 0) {
    nextY += rowHeight;
    nextX = 0;
    rowHeight = 0;
  }
  const panel = {
    id: nextPanelID++, title, type: "row", collapsed: false,
    gridPos: { h: 1, w: 24, x: 0, y: nextY++ }, panels: [],
  };
  return panel;
}

function environmentVariable() {
  return {
    name: "environment", label: "Environment", type: "custom",
    query: "production,staging", current: { text: "production", value: "production" },
    options: [
      { selected: true, text: "production", value: "production" },
      { selected: false, text: "staging", value: "staging" },
    ],
  };
}

function supportDatasourceVariable() {
  return {
    name: "support_datasource", label: "Support database", type: "datasource",
    query: "grafana-postgresql-datasource", regex: "/Hivy Support - $environment/i",
    current: { text: "Hivy Support - Production", value: "hivy-support-production" },
    refresh: 1,
  };
}

function textVariable(name, label) {
  return { name, label, type: "textbox", query: "", current: { text: "", value: "" } };
}

function dashboard({ title, uid, description, variables = [], panels }) {
  nextPanelID = 1;
  nextY = 0;
  nextX = 0;
  rowHeight = 0;
  const builtPanels = panels();
  return {
    annotations: {
      list: [{
        builtIn: 1, datasource: { type: "grafana", uid: "-- Grafana --" },
        enable: true, hide: true, iconColor: "rgba(0, 211, 255, 1)",
        name: "Annotations & Alerts", type: "dashboard",
      }],
    },
    description,
    editable: false,
    fiscalYearStartMonth: 0,
    graphTooltip: 1,
    links: [],
    liveNow: false,
    panels: builtPanels,
    refresh: "30s",
    schemaVersion: 39,
    tags: ["hivy", "operations"],
    templating: { list: variables },
    time: { from: "now-6h", to: "now" },
    timepicker: {},
    timezone: "browser",
    title,
    uid,
    version: 1,
    weekStart: "",
  };
}

const scopedOrg = "('$org_id' = '' OR org_id::text = '$org_id')";
const scopedSession = "('$session_id' = '' OR session_id::text = '$session_id')";
const errorLogs = 'kubernetes.pod_namespace:="$environment" AND (level:~"(?i)(error|fatal|panic)" OR _msg:~"(?i)(error|fatal|panic)")';

const dashboards = [
  {
    file: "hivy-customer-support.json",
    config: {
      title: "Hivy / Customer Support 360", uid: "hivy-customer-support",
      description: "Fast, content-safe org and session diagnosis across operational database state and logs.",
      variables: [environmentVariable(), supportDatasourceVariable(), textVariable("org_id", "Org ID"), textVariable("session_id", "Session ID")],
      panels: () => [
        row("Customer health"),
        sqlStat("Sessions (24h)", `SELECT COALESCE(SUM(sessions_24h),0) FROM observability_org_support WHERE ${scopedOrg}`),
        sqlStat("Failed sessions", `SELECT COALESCE(SUM(failed_session_count),0) FROM observability_org_support WHERE ${scopedOrg}`, { thresholds: [{ color: "green", value: null }, { color: "red", value: 1 }] }),
        sqlStat("Active connections", `SELECT COALESCE(SUM(active_connection_count),0) FROM observability_org_support WHERE ${scopedOrg}`),
        sqlStat("Credit balance", `SELECT COALESCE(SUM(credit_balance),0) FROM observability_org_support WHERE ${scopedOrg}`),
        sqlTable("Org operational profile", `SELECT * FROM observability_org_support WHERE ${scopedOrg} ORDER BY last_session_at DESC NULLS LAST LIMIT 100`, { width: 24 }),
        row("Session diagnosis"),
        sqlTable("Session state and usage", `SELECT * FROM observability_session_support WHERE ${scopedOrg} AND ${scopedSession} ORDER BY created_at DESC LIMIT 200`, { width: 24, height: 10 }),
        logPanel("Cross-service session and org timeline", `kubernetes.pod_namespace:="$environment" AND ('$session_id'="" OR session_id:="$session_id" OR _msg:~"$session_id") AND ('$org_id'="" OR org_id:="$org_id" OR _msg:~"$org_id") | fields _time, level, _msg, service, event, phase, status, duration_ms, session_id, org_id, agent_id, sandbox_id, request_id, trace_id | limit 1000`, { limit: 1000 }),
      ],
    },
  },
  {
    file: "hivy-api-reliability.json",
    config: {
      title: "Hivy / API Reliability and SLOs", uid: "hivy-api-reliability",
      description: "RED metrics for API, MCP, Microsandbox control, and runner endpoints.",
      variables: [environmentVariable()],
      panels: () => [
        stat("Requests / sec", 'sum(rate(hivy_http_requests_total{namespace="$environment"}[5m]))', { unit: "reqps" }),
        stat("5xx rate", 'sum(rate(hivy_http_requests_total{namespace="$environment",status_class="5xx"}[5m])) / clamp_min(sum(rate(hivy_http_requests_total{namespace="$environment"}[5m])), 0.001)', { unit: "percentunit", thresholds: [{ color: "green", value: null }, { color: "orange", value: 0.01 }, { color: "red", value: 0.05 }] }),
        stat("p95 latency", 'histogram_quantile(0.95, sum by (le) (rate(hivy_http_request_duration_seconds_bucket{namespace="$environment"}[5m])))', { unit: "s" }),
        stat("In flight", 'sum(hivy_http_requests_in_flight{namespace="$environment"})'),
        timeseries("Request rate by service", 'sum by (service) (rate(hivy_http_requests_total{namespace="$environment"}[5m]))', { unit: "reqps", legend: "{{service}}" }),
        timeseries("p95 latency by service", 'histogram_quantile(0.95, sum by (le,service) (rate(hivy_http_request_duration_seconds_bucket{namespace="$environment"}[5m])))', { unit: "s", legend: "{{service}}" }),
        timeseries("Error rate by route", 'sum by (service,route) (rate(hivy_http_requests_total{namespace="$environment",status_class=~"4xx|5xx"}[5m]))', { unit: "reqps", legend: "{{service}} {{route}}" }),
        logPanel("API and MCP errors", `${errorLogs} AND kubernetes.pod_name:~"backend-api-.*" | fields _time, level, _msg, request_id, method, path, status, latency_ms, session_id, org_id | limit 750`, { limit: 750 }),
      ],
    },
  },
  {
    file: "hivy-agent-llm.json",
    config: {
      title: "Hivy / Agent Runs and LLM", uid: "hivy-agent-llm",
      description: "Provider reliability, latency, token use, cost, and provisioning phase performance.",
      variables: [environmentVariable(), supportDatasourceVariable(), textVariable("org_id", "Org ID")],
      panels: () => [
        stat("Generations / sec", 'sum(rate(hivy_llm_generations_total{namespace="$environment"}[5m]))', { unit: "reqps" }),
        stat("Generation errors", 'sum(rate(hivy_llm_generations_total{namespace="$environment",status="error"}[5m])) / clamp_min(sum(rate(hivy_llm_generations_total{namespace="$environment"}[5m])), 0.001)', { unit: "percentunit" }),
        stat("p95 generation", 'histogram_quantile(0.95, sum by (le) (rate(hivy_llm_generation_duration_seconds_bucket{namespace="$environment"}[10m])))', { unit: "s" }),
        stat("Cost / hour", 'sum(increase(hivy_llm_cost_usd_total{namespace="$environment"}[1h]))', { unit: "currencyUSD" }),
        timeseries("Generation rate by provider and model", 'sum by (provider,model,status) (rate(hivy_llm_generations_total{namespace="$environment"}[5m]))', { unit: "reqps", legend: "{{provider}} / {{model}} / {{status}}" }),
        timeseries("Token rate", 'sum by (type) (rate(hivy_llm_tokens_total{namespace="$environment"}[5m]))', { unit: "ops", legend: "{{type}}" }),
        timeseries("Provisioning p95 by phase", 'histogram_quantile(0.95, sum by (le,domain,operation) (rate(hivy_workflow_duration_seconds_bucket{namespace="$environment"}[10m])))', { unit: "s", legend: "{{domain}} / {{operation}}" }),
        sqlTable("Hourly provider and model usage", `SELECT bucket, provider_id, model, request_count, error_count, input_tokens, output_tokens, cost, avg_ttfb_ms, avg_total_ms FROM observability_llm_hourly WHERE $__timeFilter(bucket) AND ${scopedOrg} ORDER BY bucket DESC LIMIT 500`, { width: 24 }),
        sqlTable("Hourly tool reliability", `SELECT bucket, tool_name, status, call_count, error_count, avg_total_ms, credits_used FROM observability_tool_hourly WHERE $__timeFilter(bucket) AND ${scopedOrg} ORDER BY bucket DESC, error_count DESC LIMIT 500`, { width: 24 }),
        logPanel("Agent and provider errors", `${errorLogs} AND kubernetes.pod_name:~"backend-(api|worker)-.*" | fields _time, level, _msg, event, session_id, agent_id, provider, model, status, duration_ms | limit 750`, { limit: 750 }),
      ],
    },
  },
  {
    file: "hivy-asynq.json",
    config: {
      title: "Hivy / Async Work and Queues", uid: "hivy-asynq",
      description: "Queue depth, latency, retries, archived tasks, throughput, and worker failures.",
      variables: [environmentVariable()],
      panels: () => [
        stat("Pending", 'sum(max by (queue) (hivy_asynq_queue_tasks{namespace="$environment",state="pending"}))'),
        stat("Retry", 'sum(max by (queue) (hivy_asynq_queue_tasks{namespace="$environment",state="retry"}))'),
        stat("Archived", 'sum(max by (queue) (hivy_asynq_queue_tasks{namespace="$environment",state="archived"}))', { thresholds: [{ color: "green", value: null }, { color: "red", value: 1 }] }),
        stat("Oldest pending", 'max(hivy_asynq_queue_latency_seconds{namespace="$environment"})', { unit: "s" }),
        timeseries("Queue depth by state", 'max by (queue,state) (hivy_asynq_queue_tasks{namespace="$environment"})', { legend: "{{queue}} / {{state}}", stack: true }),
        timeseries("Task throughput by type", 'sum by (queue,task_type,status) (rate(hivy_asynq_tasks_total{namespace="$environment"}[5m]))', { unit: "ops", legend: "{{queue}} / {{task_type}} / {{status}}" }),
        timeseries("p95 task duration", 'histogram_quantile(0.95, sum by (le,queue,task_type) (rate(hivy_asynq_task_duration_seconds_bucket{namespace="$environment"}[10m])))', { unit: "s", legend: "{{queue}} / {{task_type}}" }),
        logPanel("Worker errors", `${errorLogs} AND kubernetes.pod_name:~"backend-worker-.*" | fields _time, level, _msg, task_type, queue, retry_count, session_id, org_id, sandbox_id | limit 750`, { limit: 750 }),
      ],
    },
  },
  {
    file: "hivy-sandbox-capacity.json",
    config: {
      title: "Hivy / Sandbox Capacity and Lifecycle", uid: "hivy-sandbox-capacity",
      description: "Runner health, capacity, saturation, sandbox lifecycle state, and provisioning errors.",
      variables: [environmentVariable(), textVariable("runner_id", "Runner ID")],
      panels: () => [
        stat("Healthy runners", 'sum(hivy_microsandbox_runner_status{status="healthy"})'),
        stat("Unhealthy runners", 'sum(hivy_microsandbox_runner_status{status="unhealthy"})', { thresholds: [{ color: "green", value: null }, { color: "red", value: 1 }] }),
        stat("Running sandboxes", 'sum(hivy_microsandbox_sandboxes{status="running"})'),
        stat("Max heartbeat age", 'max(hivy_microsandbox_runner_heartbeat_age_seconds)', { unit: "s" }),
        timeseries("Sandbox lifecycle state", 'sum by (runner_id,status) (hivy_microsandbox_sandboxes)', { legend: "{{runner_id}} / {{status}}", stack: true }),
        timeseries("Runner reserved capacity", 'hivy_microsandbox_runner_reserved / clamp_min(hivy_microsandbox_runner_capacity, 1)', { unit: "percentunit", legend: "{{runner_id}} / {{resource}}" }),
        timeseries("Runner pressure", 'hivy_microsandbox_runner_pressure', { legend: "{{runner_id}} / {{signal}}" }),
        timeseries("Provisioning phase p95", 'histogram_quantile(0.95, sum by (le,domain,operation) (rate(hivy_workflow_duration_seconds_bucket{domain=~".*(sandbox|runtime).*"}[10m])))', { unit: "s", legend: "{{domain}} / {{operation}}" }),
        logPanel("Control, runner, and sandbox errors", `(source:~"runner|sandbox" OR kubernetes.pod_name:~"microsandbox-.*") AND ('$runner_id'="" OR runner_id:="$runner_id") AND (level:~"(?i)(error|fatal|panic)" OR _msg:~"(?i)(error|fatal|panic)") | fields _time, level, _msg, source, runner_id, sandbox_id, session_id, org_id, phase, duration_ms | limit 1000`, { limit: 1000 }),
      ],
    },
  },
  {
    file: "hivy-automation.json",
    config: {
      title: "Hivy / Automations and Integrations", uid: "hivy-automation",
      description: "Schedules, trigger deliveries, connections, automation latency, and delivery errors.",
      variables: [environmentVariable(), supportDatasourceVariable(), textVariable("org_id", "Org ID")],
      panels: () => [
        sqlStat("Runs", `SELECT COUNT(*) FROM observability_automation_runs WHERE $__timeFilter(created_at) AND ${scopedOrg}`),
        sqlStat("Schedule failures", `SELECT COUNT(*) FROM observability_automation_runs WHERE $__timeFilter(created_at) AND ${scopedOrg} AND automation_type='schedule' AND has_error`, { thresholds: [{ color: "green", value: null }, { color: "red", value: 1 }] }),
        sqlStat("Trigger deliveries", `SELECT COUNT(*) FROM observability_automation_runs WHERE $__timeFilter(created_at) AND ${scopedOrg} AND automation_type='trigger'`),
        sqlStat("p95 schedule duration", `SELECT COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms),0) FROM observability_automation_runs WHERE $__timeFilter(created_at) AND ${scopedOrg} AND duration_ms IS NOT NULL`, { unit: "ms" }),
        sqlTable("Recent automation runs", `SELECT automation_type, run_id, org_id, agent_id, automation_id, session_id, status, has_error, duration_ms, created_at FROM observability_automation_runs WHERE $__timeFilter(created_at) AND ${scopedOrg} ORDER BY created_at DESC LIMIT 500`, { width: 24 }),
        logPanel("Automation and integration errors", `${errorLogs} AND kubernetes.pod_name:~"(backend-worker|nango)-.*" AND ('$org_id'="" OR org_id:="$org_id" OR _msg:~"$org_id") | fields _time, level, _msg, event, task_type, session_id, org_id, agent_id, connection_id | limit 750`, { limit: 750 }),
      ],
    },
  },
  {
    file: "hivy-rag.json",
    config: {
      title: "Hivy / Knowledge and RAG", uid: "hivy-rag",
      description: "RAG source freshness, indexing progress, failures, Qdrant availability, and worker diagnostics.",
      variables: [environmentVariable(), supportDatasourceVariable(), textVariable("org_id", "Org ID")],
      panels: () => [
        sqlStat("Enabled sources", `SELECT COUNT(*) FROM observability_rag_health WHERE ${scopedOrg} AND enabled`),
        sqlStat("Repeated-error sources", `SELECT COUNT(*) FROM observability_rag_health WHERE ${scopedOrg} AND in_repeated_error_state`, { thresholds: [{ color: "green", value: null }, { color: "red", value: 1 }] }),
        sqlStat("Unresolved indexing errors", `SELECT COALESCE(SUM(unresolved_error_count),0) FROM observability_rag_health WHERE ${scopedOrg}`, { thresholds: [{ color: "green", value: null }, { color: "red", value: 1 }] }),
        sqlStat("Indexed documents", `SELECT COALESCE(SUM(total_docs_indexed),0) FROM observability_rag_health WHERE ${scopedOrg}`),
        sqlTable("Source health", `SELECT * FROM observability_rag_health WHERE ${scopedOrg} ORDER BY in_repeated_error_state DESC, last_successful_index_time ASC NULLS FIRST LIMIT 500`, { width: 24 }),
        timeseries("Qdrant scrape availability", 'max by (namespace,instance) (up{namespace="$environment",job=~".*qdrant.*"})', { legend: "{{namespace}} / {{instance}}" }),
        logPanel("RAG and Qdrant errors", `${errorLogs} AND (kubernetes.pod_name:~"(backend-worker|qdrant)-.*" OR _msg:~"(?i)(rag|index|embedding|qdrant)") AND ('$org_id'="" OR org_id:="$org_id" OR _msg:~"$org_id") | fields _time, level, _msg, task_type, source_id, org_id, agent_id | limit 750`, { limit: 750 }),
      ],
    },
  },
  {
    file: "hivy-data-services.json",
    config: {
      title: "Hivy / Data Services", uid: "hivy-data-services",
      description: "PostgreSQL, Redis, Qdrant, volume, and data-service workload health.",
      variables: [environmentVariable()],
      panels: () => [
        stat("PostgreSQL collectors up", 'sum(cnpg_collector_up{namespace="$environment"})'),
        stat("Redis exporter up", 'max(redis_up{namespace="$environment"})', { thresholds: [{ color: "red", value: null }, { color: "green", value: 1 }] }),
        stat("Qdrant ready pods", 'sum(kube_pod_status_ready{namespace="$environment",pod=~"qdrant-.*",condition="true"})'),
        stat("PVC max usage", 'max(kubelet_volume_stats_used_bytes{namespace="$environment"} / clamp_min(kubelet_volume_stats_capacity_bytes{namespace="$environment"}, 1))', { unit: "percentunit" }),
        timeseries("PostgreSQL replication lag", 'cnpg_pg_replication_lag{namespace="$environment"}', { unit: "s", legend: "{{pod}}" }),
        timeseries("Redis memory and clients", 'redis_memory_used_bytes{namespace="$environment"} or redis_connected_clients{namespace="$environment"}', { legend: "{{instance}}" }),
        timeseries("Data-service memory", 'sum by (pod) (container_memory_working_set_bytes{namespace="$environment",pod=~"(backend-postgres|backend-redis|qdrant)-.*",container!=""})', { unit: "bytes", legend: "{{pod}}" }),
        timeseries("PVC usage", 'kubelet_volume_stats_used_bytes{namespace="$environment"} / clamp_min(kubelet_volume_stats_capacity_bytes{namespace="$environment"}, 1)', { unit: "percentunit", legend: "{{persistentvolumeclaim}}" }),
        logPanel("Data-service errors", `${errorLogs} AND kubernetes.pod_name:~"(backend-postgres|backend-redis|qdrant)-.*" | fields _time, level, _msg, kubernetes.pod_name, kubernetes.container_name | limit 750`, { limit: 750 }),
      ],
    },
  },
  {
    file: "hivy-billing-security.json",
    config: {
      title: "Hivy / Billing and Security", uid: "hivy-billing-security",
      description: "Credit and billing integrity plus aggregated, content-safe security audit activity.",
      variables: [environmentVariable(), supportDatasourceVariable(), textVariable("org_id", "Org ID")],
      panels: () => [
        sqlStat("Unbilled generations", `SELECT COALESCE(SUM(unbilled_generation_count),0) FROM observability_billing_health WHERE ${scopedOrg}`, { thresholds: [{ color: "green", value: null }, { color: "orange", value: 1 }] }),
        sqlStat("Billing errors", `SELECT COALESCE(SUM(billing_error_count),0) FROM observability_billing_health WHERE ${scopedOrg}`, { thresholds: [{ color: "green", value: null }, { color: "red", value: 1 }] }),
        sqlStat("Pending purchases", `SELECT COALESCE(SUM(pending_purchase_count),0) FROM observability_billing_health WHERE ${scopedOrg}`),
        sqlStat("Failed purchases", `SELECT COALESCE(SUM(failed_purchase_count),0) FROM observability_billing_health WHERE ${scopedOrg}`, { thresholds: [{ color: "green", value: null }, { color: "red", value: 1 }] }),
        sqlTable("Billing health by org", `SELECT * FROM observability_billing_health WHERE ${scopedOrg} ORDER BY billing_error_count DESC, unbilled_generation_count DESC LIMIT 500`, { width: 24 }),
        sqlTable("Security event aggregates", `SELECT bucket, org_id, action, event_count, credential_count FROM observability_security_events WHERE $__timeFilter(bucket) AND ${scopedOrg} ORDER BY bucket DESC LIMIT 500`, { width: 24 }),
        logPanel("Auth, access, and billing errors", `${errorLogs} AND kubernetes.pod_name:~"backend-(api|worker)-.*" AND _msg:~"(?i)(auth|token|access|billing|credit|payment)" AND ('$org_id'="" OR org_id:="$org_id" OR _msg:~"$org_id") | fields _time, level, _msg, request_id, org_id, action, status | limit 750`, { limit: 750 }),
      ],
    },
  },
  {
    file: "hivy-product-operations.json",
    config: {
      title: "Hivy / Apps, Sheets, and Email", uid: "hivy-product-operations",
      description: "Operational status and failures for apps, sheet imports, and agent email without customer content.",
      variables: [environmentVariable(), supportDatasourceVariable(), textVariable("org_id", "Org ID")],
      panels: () => [
        sqlStat("App operations", `SELECT COUNT(*) FROM observability_product_operations WHERE $__timeFilter(created_at) AND ${scopedOrg} AND product='app'`),
        sqlStat("Sheet imports", `SELECT COUNT(*) FROM observability_product_operations WHERE $__timeFilter(created_at) AND ${scopedOrg} AND product='sheet_import'`),
        sqlStat("Email operations", `SELECT COUNT(*) FROM observability_product_operations WHERE $__timeFilter(created_at) AND ${scopedOrg} AND product='email'`),
        sqlStat("Product failures", `SELECT COUNT(*) FROM observability_product_operations WHERE $__timeFilter(created_at) AND ${scopedOrg} AND has_error`, { thresholds: [{ color: "green", value: null }, { color: "red", value: 1 }] }),
        sqlTable("Recent product operations", `SELECT product, resource_id, org_id, team_id, session_id, status, has_error, created_at, updated_at FROM observability_product_operations WHERE $__timeFilter(created_at) AND ${scopedOrg} ORDER BY created_at DESC LIMIT 500`, { width: 24 }),
        logPanel("Apps, sheets, and email errors", `${errorLogs} AND kubernetes.pod_name:~"backend-(api|worker)-.*" AND _msg:~"(?i)(app|sheet|csv|email|resend|webhook)" AND ('$org_id'="" OR org_id:="$org_id" OR _msg:~"$org_id") | fields _time, level, _msg, task_type, session_id, org_id, app_id, sheet_id | limit 750`, { limit: 750 }),
      ],
    },
  },
  {
    file: "hivy-deployments.json",
    config: {
      title: "Hivy / Deployments and Change Correlation", uid: "hivy-deployments",
      description: "Rollout state, replica gaps, image revisions, restarts, and errors around changes.",
      variables: [environmentVariable()],
      panels: () => [
        stat("Unavailable replicas", 'sum(kube_deployment_status_replicas_unavailable{namespace="$environment"})', { thresholds: [{ color: "green", value: null }, { color: "red", value: 1 }] }),
        stat("Rollouts in progress", 'count((kube_deployment_metadata_generation{namespace="$environment"} != on(namespace,deployment) kube_deployment_status_observed_generation{namespace="$environment"}) == 1)'),
        stat("Restarts (1h)", 'sum(increase(kube_pod_container_status_restarts_total{namespace="$environment"}[1h]))'),
        stat("Unready pods", 'sum(kube_pod_status_ready{namespace="$environment",condition="false"})'),
        timeseries("Desired versus available replicas", 'kube_deployment_spec_replicas{namespace="$environment"} or kube_deployment_status_replicas_available{namespace="$environment"}', { legend: "{{deployment}}" }),
        timeseries("Container restarts", 'sum by (pod,container) (increase(kube_pod_container_status_restarts_total{namespace="$environment"}[15m]))', { legend: "{{pod}} / {{container}}" }),
        logPanel("Deployment-adjacent errors", `${errorLogs} | fields _time, level, _msg, kubernetes.pod_name, kubernetes.container_name, request_id, session_id | limit 1000`, { limit: 1000 }),
      ],
    },
  },
  {
    file: "hivy-backups.json",
    config: {
      title: "Hivy / Backups and Recovery Readiness", uid: "hivy-backups",
      description: "Backup freshness, failures, scheduled job health, and storage headroom.",
      variables: [environmentVariable()],
      panels: () => [
        stat("Failed backup jobs (24h)", 'sum(increase(kube_job_status_failed{namespace="$environment",job_name=~".*backup.*"}[24h]))', { thresholds: [{ color: "green", value: null }, { color: "red", value: 1 }] }),
        stat("Active backup jobs", 'sum(kube_job_status_active{namespace="$environment",job_name=~".*backup.*"})'),
        stat("PVC max usage", 'max(kubelet_volume_stats_used_bytes{namespace="$environment"} / clamp_min(kubelet_volume_stats_capacity_bytes{namespace="$environment"}, 1))', { unit: "percentunit" }),
        stat("CronJobs suspended", 'sum(kube_cronjob_spec_suspend{namespace="$environment",cronjob=~".*backup.*"})'),
        timeseries("Backup job failures", 'sum by (job_name) (increase(kube_job_status_failed{namespace="$environment",job_name=~".*backup.*"}[1h]))', { legend: "{{job_name}}" }),
        timeseries("Backup job runtime", 'time() - kube_job_status_start_time{namespace="$environment",job_name=~".*backup.*"}', { unit: "s", legend: "{{job_name}}" }),
        logPanel("Backup errors", `${errorLogs} AND kubernetes.pod_name:~".*backup.*" | fields _time, level, _msg, kubernetes.pod_name, kubernetes.container_name | limit 750`, { limit: 750 }),
      ],
    },
  },
  {
    file: "hivy-observability-pipeline.json",
    config: {
      title: "Hivy / Observability Pipeline", uid: "hivy-observability-pipeline",
      description: "Scrape health, ingestion throughput, buffering, storage, and alert evaluation health.",
      variables: [],
      panels: () => [
        stat("Failed scrapes", 'count(up == 0)', { thresholds: [{ color: "green", value: null }, { color: "red", value: 1 }] }),
        stat("Metrics ingestion / sec", 'sum(rate(vm_rows_inserted_total[5m]))', { unit: "ops" }),
        stat("Logs ingestion / sec", 'sum(rate(vm_rows_inserted_total{job=~".*victoria.*logs.*"}[5m]))', { unit: "ops" }),
        stat("Active alerts", 'sum(ALERTS{alertstate="firing"})', { thresholds: [{ color: "green", value: null }, { color: "orange", value: 1 }] }),
        timeseries("Scrape availability by job", 'avg by (job) (up)', { unit: "percentunit", legend: "{{job}}" }),
        timeseries("Remote-write backlog", 'sum by (pod) (vmagent_remotewrite_pending_data_bytes)', { unit: "bytes", legend: "{{pod}}" }),
        timeseries("Victoria storage disk use", 'sum by (pod) (vm_data_size_bytes)', { unit: "bytes", legend: "{{pod}}" }),
        logPanel("Monitoring stack errors", `kubernetes.pod_namespace:="observability" AND (level:~"(?i)(error|fatal|panic)" OR _msg:~"(?i)(error|fatal|panic)") | fields _time, level, _msg, kubernetes.pod_name, kubernetes.container_name | limit 750`, { limit: 750 }),
      ],
    },
  },
  {
    file: "hivy-product-journeys.json",
    config: {
      title: "Hivy / Product Journey SLOs", uid: "hivy-product-journeys",
      description: "Server-side journey health for session creation, provisioning, message delivery, and agent execution.",
      variables: [environmentVariable(), supportDatasourceVariable(), textVariable("org_id", "Org ID")],
      panels: () => [
        sqlStat("Sessions created", `SELECT COUNT(*) FROM observability_session_support WHERE $__timeFilter(created_at) AND ${scopedOrg}`),
        sqlStat("Sessions with errors", `SELECT COUNT(*) FROM observability_session_support WHERE $__timeFilter(created_at) AND ${scopedOrg} AND (failed_message_count > 0 OR generation_error_count > 0 OR agent_turn_last_outcome IN ('failed','error'))`, { thresholds: [{ color: "green", value: null }, { color: "orange", value: 1 }] }),
        stat("Public probes up", 'sum(probe_success{job="hivy-public-journeys"})'),
        stat("Public probe p95", 'max(probe_duration_seconds{job="hivy-public-journeys"})', { unit: "s" }),
        stat("Session-create p95", 'histogram_quantile(0.95, sum by (le) (rate(hivy_http_request_duration_seconds_bucket{namespace="$environment",route=~".*/sessions.*",method="POST"}[10m])))', { unit: "s" }),
        stat("Provisioning p95", 'histogram_quantile(0.95, sum by (le) (rate(hivy_workflow_duration_seconds_bucket{namespace="$environment",domain=~".*provision.*|.*sandbox.*"}[10m])))', { unit: "s" }),
        timeseries("Public journey availability", 'probe_success{job="hivy-public-journeys"}', { unit: "percentunit", legend: "{{instance}}" }),
        timeseries("Public journey duration", 'probe_duration_seconds{job="hivy-public-journeys"}', { unit: "s", legend: "{{instance}}" }),
        timeseries("Journey request outcomes", 'sum by (route,status_class) (rate(hivy_http_requests_total{namespace="$environment",route=~".*(sessions|messages|sandboxes).*"}[5m]))', { unit: "reqps", legend: "{{route}} / {{status_class}}" }),
        timeseries("Journey phase p95", 'histogram_quantile(0.95, sum by (le,domain,operation) (rate(hivy_workflow_duration_seconds_bucket{namespace="$environment"}[10m])))', { unit: "s", legend: "{{domain}} / {{operation}}" }),
        sqlTable("Recent journey failures", `SELECT session_id, org_id, team_id, agent_id, sandbox_id, source, status, agent_turn_status, agent_turn_last_outcome, failed_message_count, generation_error_count, created_at, updated_at FROM observability_session_support WHERE $__timeFilter(created_at) AND ${scopedOrg} AND (failed_message_count > 0 OR generation_error_count > 0 OR agent_turn_last_outcome IN ('failed','error')) ORDER BY created_at DESC LIMIT 500`, { width: 24 }),
        logPanel("Journey errors", `${errorLogs} AND kubernetes.pod_name:~"(backend-(api|worker)|microsandbox-.*)" AND ('$org_id'="" OR org_id:="$org_id" OR _msg:~"$org_id") | fields _time, level, _msg, event, phase, status, duration_ms, session_id, org_id, agent_id, sandbox_id | limit 1000`, { limit: 1000 }),
      ],
    },
  },
];

for (const { file, config } of dashboards) {
  const contents = JSON.stringify(dashboard(config), null, 2) + "\n";
  writeFileSync(resolve(outputDir, file), contents);
}

console.log(`generated ${dashboards.length} dashboards in ${outputDir}`);
