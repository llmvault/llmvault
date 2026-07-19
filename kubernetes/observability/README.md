# Cluster observability

The shared `observability` namespace contains the cluster-wide monitoring and
logging stack. The deployment uses the official
`victoria-metrics-k8s-stack` Helm chart, pinned to version `0.86.2`.

The stack provides:

- VictoriaMetrics with 30-day retention and a 15 GiB expandable Longhorn PVC;
- VictoriaLogs with 14-day retention and a 30 GiB expandable Longhorn PVC;
- one VLAgent per node, reading every Kubernetes container log with a
  persistent 1 GB node-local outage buffer;
- VMAgent, kube-state-metrics, kubelet/cAdvisor, CoreDNS, API server, and node
  exporter metrics;
- Grafana dashboards and VictoriaMetrics alert rules;
- Hivy environment and service dashboards, with `production` as the default
  environment and one-click drill-down from service health to metrics and logs;
- VMAlert and Alertmanager, initially using a dashboard-only receiver;
- an authenticated HTTPS dashboard at `monitor.usehivy.com`.

Only Grafana is attached to the public Gateway. VictoriaMetrics,
VictoriaLogs, VMAlert, Alertmanager, and the collectors remain private
ClusterIP endpoints.

## Secrets

Generate the ignored Grafana administrator input:

```sh
kubernetes/observability/generate-secrets.sh
```

Create or refresh the Kubernetes Secret before installing the chart:

```sh
kubectl create namespace observability --dry-run=client -o yaml | kubectl apply -f -
kubectl -n observability create secret generic observability-grafana-admin \
  --from-env-file=kubernetes/observability/secrets/grafana-admin.env \
  --dry-run=client -o yaml | kubectl apply -f -
```

## Render and deploy

```sh
helm template obs vm/victoria-metrics-k8s-stack \
  --version 0.86.2 \
  --namespace observability \
  --include-crds \
  --values kubernetes/observability/values.yaml \
  > /tmp/observability.yaml

kubectl apply --dry-run=server -f /tmp/observability.yaml

helm upgrade --install obs vm/victoria-metrics-k8s-stack \
  --version 0.86.2 \
  --namespace observability \
  --values kubernetes/observability/values.yaml \
  --wait --timeout 15m

kubectl apply -k kubernetes/observability
kubectl apply -f kubernetes/ingress/public/certificate.yaml
```

Alert notifications deliberately remain dashboard-only until a notification
destination is selected. Add Slack, email, or another receiver to
`alertmanager.config` without exposing Alertmanager publicly.

## Hivy dashboards

Grafana opens on **Hivy / Environment Overview**. Use the environment selector
to switch between `production` and `staging`, then click a service health card
to open **Hivy / Service Details** with that service preselected. The detail
dashboard provides pod health, CPU, memory, network traffic, restarts, log
volume, error volume, persistent-volume usage, and the application log stream.

The service selector covers Web, API, Asynq, Nango, PostgreSQL, Redis, Qdrant,
Microsandbox, and Zot. A service that is intentionally absent from an
environment is shown as `NOT DEPLOYED` on the environment overview.
