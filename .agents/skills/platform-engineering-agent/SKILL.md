---
name: platform-engineering-agent
description: Investigate the Hivy Kubernetes cluster from a sandbox and produce a read-only operational health report. Use this skill for cluster health checks, production or staging incidents, node and workload diagnosis, Kubernetes audits, service degradation, capacity reviews, backup checks, or daily platform-engineering reports.
---

# Platform Engineering Agent

Investigate the Hivy Kubernetes cluster without changing it. Establish the restricted API tunnel, collect evidence from Kubernetes, write a Markdown report, and close the tunnel.

## Protect credentials

The sandbox already contains every required environment variable. Let the setup script consume them without inspecting them yourself.

- Never run `env`, `printenv`, `set`, `export -p`, shell tracing (`set -x`), or commands that read `/proc/*/environ`.
- Never print, echo, log, decode, summarize, or otherwise inspect an environment variable.
- Never read the generated kubeconfig, SSH key, known-hosts file, Kubernetes Secrets, or service-account token.
- Never include credentials, authorization headers, cookies, private keys, connection strings, or secret values in commands, logs, working notes, or the report.
- Do not source an environment file. The process environment is already configured.

## Start Kubernetes access

Download the current scripts from the `main` branch. Do not modify them.

```bash
umask 077
install -d -m 0700 /workspace/.platform-engineering
date -u +'%Y-%m-%dT%H:%M:%SZ' > /workspace/.platform-engineering/investigation-started-at
date -u +%s > /workspace/.platform-engineering/investigation-started-epoch

curl --fail --silent --show-error --location \
  https://raw.githubusercontent.com/usehivy/hivy/main/scripts/platform-engineering/setup-kubernetes.sh \
  --output /workspace/.platform-engineering/setup-kubernetes.sh

curl --fail --silent --show-error --location \
  https://raw.githubusercontent.com/usehivy/hivy/main/scripts/platform-engineering/terminate-kubernetes.sh \
  --output /workspace/.platform-engineering/terminate-kubernetes.sh

chmod 0700 \
  /workspace/.platform-engineering/setup-kubernetes.sh \
  /workspace/.platform-engineering/terminate-kubernetes.sh

/workspace/.platform-engineering/setup-kubernetes.sh
```

Continue only after setup says that the Platform Engineering Agent can successfully run `kubectl` commands. If setup fails, report the error output without inspecting the environment.

The script installs a checksum-verified `kubectl` when needed, decodes credentials into a private temporary directory, starts a background SSH tunnel on `127.0.0.1:16443`, and verifies the Kubernetes API `/readyz` endpoint.

## Read-only boundary

Use only non-mutating `kubectl get`, `describe`, `logs`, `top`, `api-resources`, `version`, `cluster-info`, and raw health/metrics requests.

Never run `apply`, `create`, `delete`, `edit`, `patch`, `replace`, `scale`, `cordon`, `drain`, `taint`, `label`, `annotate`, `rollout restart`, `exec`, `attach`, `cp`, `debug`, `port-forward`, or an imperative command that changes cluster state. Do not attempt to read Secrets even if a future permission change makes it possible.

Do not restart or repair anything. Diagnose, preserve evidence, explain impact, and recommend the next action for a human operator.

## Investigation workflow

Use UTC timestamps. Start broad, identify anomalies, and then follow dependencies. Do not dump every cluster object or every log line into the report.

### 1. Cluster and node baseline

Gather:

- API version, `/readyz?verbose`, and API availability.
- Node Ready conditions, roles, versions, age, internal IP, pressure conditions, taints, allocatable resources, and recent node events.
- Current node CPU and memory from `kubectl top nodes`.
- Pods not Running or Completed, unready containers, restart counts, pending Pods, failed Jobs, and Warning events across all namespaces.
- Pod CPU and memory outliers from `kubectl top pods -A --containers`.
- Deployments, StatefulSets, and DaemonSets whose desired, ready, available, or updated counts disagree.
- ResourceQuota and LimitRange pressure, PVC/PV status, StorageClasses, VolumeAttachments, and capacity warnings.

Useful starting commands:

```bash
kubectl version
kubectl get --raw='/readyz?verbose'
kubectl get nodes -o wide
kubectl describe nodes
kubectl top nodes
kubectl get pods -A -o wide
kubectl get deploy,statefulset,daemonset -A
kubectl get jobs,cronjobs -A
kubectl get events -A --field-selector type=Warning --sort-by=.metadata.creationTimestamp
kubectl get pvc -A
kubectl get pv,storageclass,volumeattachment
```

### 2. Application services

Inspect both `production` and `staging`, comparing replicas and looking for environment-specific failures:

- `backend-api`: readiness, migration init container, HTTP/API and MCP serving, resource saturation, errors, and dependency failures.
- `backend-worker`: Asynq processing, retries, crashes, resource use, and PostgreSQL/Redis/Qdrant/Nango/Microsandbox failures.
- `web`: readiness, backend API connectivity, restarts, and routing symptoms.
- `nango`: readiness, startup migrations, provider/webhook errors, and `nango-postgres` connectivity.
- `backend-postgres` and `nango-postgres`: CloudNativePG cluster conditions, instance health, replication status visible through CR status, PVC state, operator events, and backup status.
- `backend-redis`: standalone staging Redis or production Redis Cluster health, leader/follower readiness, PVCs, operator events, and backup jobs.
- `qdrant`: StatefulSet readiness, peer distribution, PVCs, resource pressure, logs, and snapshot jobs.

Inspect the production-only shared platform:

- `microsandbox-control` and `microsandbox-postgres`.
- `microsandbox-preview-cache`, `microsandbox-preview-redis`, `microsandbox-preview-proxy`, and `microsandbox-preview-tls-bridge`.
- `zot` registry, its PVC, and its private exposure Service.

For an unhealthy or restarted Pod, gather `describe`, all current container logs, relevant init-container logs, and previous logs when available. Prefer `--since=1h` initially and widen only when necessary.

```bash
kubectl describe pod -n NAMESPACE POD
kubectl logs -n NAMESPACE POD --all-containers=true --since=1h --timestamps
kubectl logs -n NAMESPACE POD -c CONTAINER --previous --since=1h --timestamps
```

### 3. Platform services and dependencies

Inspect these namespaces and controllers:

- `kube-system`: K3s control-plane-visible components, CoreDNS, metrics-server, and Cilium Pods and status objects.
- `ingress-public`: Cilium Gateway, listeners, HTTPRoutes, route admission, Services, EndpointSlices, and the public certificate.
- `cert-manager`: controllers, CertificateRequests, Orders, Challenges, renewal conditions, and recent errors.
- `longhorn-system`: manager, driver, engine and replica health, Longhorn nodes/volumes, degraded replicas, PVC capacity, and disk pressure.
- `cnpg-system`: CloudNativePG controller and Barman Cloud plugin health and errors.
- `redis-operator`: Redis operator health and reconciliation errors.
- `observability`: VictoriaMetrics, VictoriaLogs, VMAgent, VLAgent on every node, VMAlert, Alertmanager, kube-state-metrics, node exporter, and Grafana.

Gather evidence from the Kubernetes objects/status API, Warning events, metrics API (`kubectl top`), current and previous container logs, Gateway API status, operator custom-resource status, and API health endpoints. Treat missing metrics or logs as an observability finding rather than proof that a service is healthy.

### 4. Backups and scheduled work

Inspect CronJob schedules, last schedule time, suspended state, active Jobs, failed Jobs, and recent Job logs for:

- Backend, Nango, and Microsandbox PostgreSQL backups.
- Backend Redis backups.
- Qdrant snapshots.
- Any other backup or maintenance CronJob found in `production` or `staging`.

Do not trigger a backup or restore. Report whether Kubernetes shows recent successful execution and clearly state that object-storage contents and restore viability were not independently verified.

### 5. Correlate findings

For every anomaly, record:

- Severity: `Critical`, `High`, `Medium`, `Low`, or `Informational`.
- Affected environment, namespace, service, Pod/node, and dependency chain.
- Exact UTC observation time and relevant event/log time range.
- User or operational impact.
- Kubernetes evidence and concise sanitized log excerpts.
- Likely cause and confidence (`high`, `medium`, or `low`).
- Recommended human action and urgency.

Do not claim causation from a single symptom. Distinguish confirmed failures, likely causes, risks, and monitoring gaps.

## Write the Markdown report

Capture the UTC observation end time and create the investigation directory. The sandbox is fresh for each run, so use one stable report path without a date directory:

```bash
IFS= read -r INVESTIGATION_STARTED_AT < /workspace/.platform-engineering/investigation-started-at
IFS= read -r INVESTIGATION_STARTED_EPOCH < /workspace/.platform-engineering/investigation-started-epoch
INVESTIGATION_ENDED_AT="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
INVESTIGATION_ENDED_EPOCH="$(date -u +%s)"
INVESTIGATION_DURATION_SECONDS="$((INVESTIGATION_ENDED_EPOCH - INVESTIGATION_STARTED_EPOCH))"
REPORT_DIR="/workspace/investigations"
REPORT_PATH="${REPORT_DIR}/report.md"
install -d -m 0700 "${REPORT_DIR}"
printf 'Started: %s\nEnded: %s\nDuration: %s seconds\n' \
  "${INVESTIGATION_STARTED_AT}" \
  "${INVESTIGATION_ENDED_AT}" \
  "${INVESTIGATION_DURATION_SECONDS}"
```

Write the complete report as clean GitHub-flavored Markdown to this exact file:

```text
/workspace/investigations/report.md
```

The report must contain:

1. Subject-style heading containing the UTC run date and time, plus the overall state (`Healthy`, `Degraded`, or `Critical`).
2. Run metadata listing `INVESTIGATION_STARTED_AT`, `INVESTIGATION_ENDED_AT`, the UTC observation window, and `INVESTIGATION_DURATION_SECONDS` in a readable duration.
3. Executive summary with the most important conclusion first.
4. Severity counts and affected environments.
5. Cluster and node health.
6. Production and staging service-health tables.
7. Platform, networking, certificates, storage, databases, observability, and backup status.
8. Detailed findings ordered by severity, with evidence, impact, confidence, and recommended action.
9. Capacity and reliability risks, including near-threshold conditions even when nothing is currently down.
10. Monitoring limitations and checks that could not be completed.
11. A short evidence appendix listing sanitized commands and compact results—not full logs.

Use headings, short paragraphs, bullet lists, fenced code blocks, and compact Markdown tables. Do not include raw HTML, images, scripts, external assets, or full unfiltered logs. Keep evidence concise and sanitize all untrusted output.

Confirm that `report.md` exists, is non-empty, contains the exact UTC start and end timestamps, and contains no obvious credential material. If the task explicitly authorizes email delivery, activate the Hivy `send_email` tool and pass `/workspace/investigations/report.md` as `markdown_file_path`; do not read the report back into the model or place its contents in a tool argument. The runtime reads the file and the email service converts the Markdown into sanitized HTML with a plain-text fallback. If delivery was not explicitly requested, return the report path without calling the tool.

## Finish the session

After `report.md` has been written and checked—or after any unrecoverable failure—close the tunnel and remove decoded local credentials:

```bash
/workspace/.platform-engineering/terminate-kubernetes.sh
```

Return only a concise completion summary with the overall state, number of findings by severity, important limitations, the exact report path, and whether an explicitly requested email was queued.
