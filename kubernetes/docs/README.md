# Hivy Kubernetes operations

Hivy runs on one K3s control-plane server and one dedicated Microsandbox runner
on Hetzner bare metal. Only the production application environment is deployed;
the cluster operators, ingress, storage, observability, and Microsandbox control
plane support that environment.

No GitOps controller owns this cluster. Operators and application manifests are
applied with `kubectl`; a few pinned charts use Helm either as a renderer or as
an installed release. Ansible prepares hosts and installs K3s, but it doesn't
deploy application workloads.

## Read by task

| Need | Document |
| --- | --- |
| Understand the private vSwitch, public load balancer, DNS, Gateway listeners, or sandbox paths | [Networking](networking.md) |
| Prepare a bare-metal host, install K3s, add a node, or manage runners | [Ansible and hosts](ansible-and-hosts.md) |
| Install Gateway API, Cilium, cert-manager, or the Vercel DNS solver | [Cluster bootstrap](cluster-bootstrap.md) |
| Install or update Longhorn, CloudNativePG, Barman Cloud, or the Redis Operator | [Storage and data operators](operators.md) |
| Find an application, dependency, Service, route, or namespace | [Applications](applications.md) |
| Understand PostgreSQL, Redis, Qdrant, Longhorn, S3 backups, and restore checks | [Data services and backups](data-services-and-backups.md) |
| Deploy main to production, publish a release, verify a rollout, or roll back an image | [Deployments and CI](deployments-and-ci.md) |
| Restore the local infrastructure configuration folder or rotate a Kubernetes Secret | [Configuration and secrets](configuration-and-secrets.md) |
| Find logs, pod metrics, node pressure, restart history, or a dashboard | [Observability](observability.md) |
| Review trust boundaries, RBAC, NetworkPolicy, TLS, and credential handling | [Security](security.md) |
| Give the external Platform Engineering Agent cluster-wide diagnostic access | [Platform Engineering Agent](platform-engineering-agent.md) |
| Diagnose a failed request, sandbox, database, DNS path, volume, or Gateway route | [Troubleshooting](troubleshooting.md) |

## Current machines

| Role | Inventory name | Public IP | Private vSwitch IP |
| --- | --- | --- | --- |
| K3s server and worker | `k8s0` | `95.217.38.240` | `10.80.1.4` |
| Microsandbox runner | `runner0` | `65.21.216.179` | `10.80.1.2` |

The Hetzner load balancer has public address `95.217.175.232`. Host traffic uses
the `10.80.0.0/16` vSwitch; the current machines
sit in `10.80.1.0/24`. K3s assigns Pods from `10.42.0.0/16` and Services from
`10.43.0.0/16`.

## Management boundaries

| Layer | How it is managed |
| --- | --- |
| Ubuntu preparation, vSwitch interface, host firewall, K3s service, runner binary and systemd service, runner CoreDNS, runner HAProxy, restricted automation tunnel users | Ansible playbooks under `ansible/` |
| Gateway API CRDs, Cilium, cert-manager and its Vercel DNS webhook | Pinned render or direct `kubectl apply` during bootstrap |
| Longhorn, CloudNativePG, Barman Cloud, Redis Operator | Pinned manifests rendered or downloaded locally, then server-side `kubectl apply` |
| Production application resources | The Kustomize overlay under `kubernetes/environments/production/` |
| Qdrant | Official chart rendered to ignored YAML, then direct `kubectl apply` |
| Zot | Helm release in `production` with an internal-only runner path |
| VictoriaMetrics, VictoriaLogs, Grafana, and collectors | Helm release in `observability`, plus local Kustomize resources |
| API, Asynq, and web image changes | Restricted GitHub Actions deployment accounts patch three Deployments |

GlitchTip remains outside this cluster. Applications send errors to its
Sentry-compatible endpoint through environment-specific DSNs; no GlitchTip
workload or database is managed by these Kubernetes manifests.

`kubernetes/config/` holds local environment files, kubeconfigs, tokens, and
rendered artifacts. Live credentials are ignored. Non-secret `*.config.env`
files and `*.env.example` templates remain in Git.

## Public hostnames

| Host | Backend |
| --- | --- |
| `usehivy.com` / `staging.usehivy.com` | Next.js web Services |
| `api.usehivy.com` / `staging.api.usehivy.com` | Go API Services |
| `proxy.usehivy.com` / `staging.proxy.usehivy.com` | Go API with `/` rewritten to `/v1/proxy/` |
| `connections.usehivy.com` / `staging.connections.usehivy.com` | Nango Services |
| `*.preview.usehivy.com` | Microsandbox preview proxy and runner host ports |
| `monitor.usehivy.com` | Grafana |

`registry.usehivy.com` isn't attached to the public load balancer. Runners reach
Zot through private DNS and their local HAProxy listener.

## Known configuration gaps

Only staging currently declares an HTTPRoute for `staging.mcp.usehivy.com`.
Neither MCP hostname appears in the shared certificate manifest, and production
has no MCP HTTPRoute. Treat MCP TLS failures as an ingress configuration issue,
not an application or provider failure, until those manifests are fixed.

GitHub Actions publishes application images and deploys the staging namespace
on pushes to `main`. Stable GitHub releases publish versioned images and deploy
the production namespace. Daytona image targets and snapshot publication remain
disabled.

## Safe first commands

```sh
export KUBECONFIG="$PWD/kubernetes/config/kubeconfigs/k8s0/local.yaml"
kubernetes/config/validate.sh
kubectl get nodes -o wide
kubectl get pods -A --field-selector=status.phase!=Running,status.phase!=Succeeded
kubectl get httproutes -A
kubectl get clusters.postgresql.cnpg.io -A
kubectl get volumes.longhorn.io -n longhorn-system
```

Commands in these docs assume the repository root as the working directory and
the local admin kubeconfig above unless a section says otherwise.
