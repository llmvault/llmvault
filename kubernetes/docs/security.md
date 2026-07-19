# Security model

This document records the security boundaries that the manifests and Ansible
roles enforce today. It also names the places where the platform trusts its
network instead of authenticating or encrypting every hop.

## Credential storage

Live infrastructure credentials belong under `kubernetes/config/`; they must
never enter Git. The ignore rules in `kubernetes/config/.gitignore` cover live
`*.env` files, kubeconfigs, credentials, and generated manifests. Only
`*.env.example` templates and non-secret `*.config.env` files are committed.

The directory contains plaintext recovery material. Keep directories at mode
`0700`, credential files at mode `0600`, and back up the whole directory to the
approved 1Password vault. Run this before applying configuration or committing:

```sh
kubernetes/config/validate.sh
git status --ignored --short kubernetes/config
git ls-files kubernetes/config
```

The final command should show templates, scripts, and committed configuration,
but no live `*.env`, kubeconfig, token, private key, or rendered Secret. Don't
use `git add -f` under `kubernetes/config`.

Kustomize reads the ignored env files and creates ordinary Kubernetes Secrets.
The generated Secret names don't carry content hashes because operators and
other manifests refer to fixed names. After changing a live env file, apply the
overlay and restart the consumers; Kubernetes won't restart a Pod merely
because data inside a fixed-name Secret changed.

K3s has `secrets-encryption: true`, so Secret values in embedded etcd are
encrypted at rest. Administrator kubeconfigs and etcd snapshots can still
recover cluster credentials; protect their ignored local copies and S3 bucket
as production secrets.

Application containers don't receive Kubernetes API credentials. The API,
worker, web, Nango, Microsandbox, preview proxy, and preview TLS bridge all set
`automountServiceAccountToken: false`.

## Cluster administration and CI access

The Kubernetes API listens on each K3s node's private address. Administrators
reach it through an SSH tunnel; the local admin kubeconfig lives at
`kubernetes/config/kubeconfigs/k8s0/local.yaml`.

GitHub Actions uses one ServiceAccount in `staging` and another in
`production`. Each namespace Role permits only `get` and `patch` on these
Deployments:

- `backend-api`
- `backend-worker`
- `web`

The accounts can't list Deployments, read Pods or Secrets, change ConfigMaps,
or touch data services. A separate Unix user exists for each environment on the
K3s nodes. Its SSH key has `restrict`, `port-forwarding`, and
`permitopen="127.0.0.1:6443"`; it doesn't grant an interactive shell. SSH only
opens the network path, while namespace RBAC decides what CI may change.

Keep the `staging` and `production` GitHub Environments separate. Never put an
administrator kubeconfig in `KUBE_CONFIG_B64`, and don't reuse the production
deployment token in staging.

## Public ingress

Public IPv4 traffic enters the Hetzner load balancer at `65.109.40.68`. The load
balancer forwards TCP without terminating TLS:

| Public port | Node port | Source allowed by node firewall |
| --- | --- | --- |
| `80` | `10080` | Hetzner LB private IP `10.80.0.4` |
| `443` | `10443` | Hetzner LB private IP `10.80.0.4` |

PROXY v2 remains enabled on both listeners. Cilium Envoy consumes that header,
terminates TLS, preserves the client address, and applies Gateway API routes.
The `http` listener redirects application hosts to HTTPS. cert-manager issues
the shared certificate through the Vercel DNS-01 webhook; the Vercel credential
lives in a Kubernetes Secret rather than a manifest.

Only IPv4 DNS records should point at the load balancer. The checked-in
bootstrap notes record a failing IPv6/PROXY-protocol combination with Cilium;
don't publish the load balancer's IPv6 address until that path passes a fresh
test.

Grafana is the sole public endpoint in `observability`. VictoriaMetrics,
VictoriaLogs, VMAlert, Alertmanager, and their collectors remain ClusterIP-only.
Grafana disables anonymous access and account creation, uses secure SameSite
cookies, and reads its administrator credential from
`observability-grafana-admin`.

Qdrant, Redis, PostgreSQL, the Microsandbox control plane, and Zot have no
public Gateway route. Zot accepts private runner traffic on NodePort `32500`;
the node firewall and Cilium policy restrict the path to the two runner IPs
currently listed in configuration.

## Private network paths

The K3s nodes and Microsandbox runners share Hetzner's private vSwitch. Current
addresses are:

| Host | Private address | Role |
| --- | --- | --- |
| `runner0` | `10.80.1.2` | sandbox runner, CoreDNS, HAProxy |
| `runner1` | `10.80.1.3` | sandbox runner, CoreDNS, HAProxy |
| `k8s-0` | `10.80.1.4` | K3s server and worker |
| `k8s-1` | `10.80.1.5` | K3s server and worker |

Runner-local CoreDNS maps `api.usehivy.com`, `staging.api.usehivy.com`,
`proxy.usehivy.com`, and `staging.proxy.usehivy.com` to that runner's HAProxy.
HAProxy passes TLS through, adds PROXY v2, and balances across nodes in the
Ansible `k3s_ingress` group. Those API and LLM-proxy requests stay on the
vSwitch while retaining normal public certificates and hostname verification.

Cluster Pods resolve `*.preview.usehivy.com` through a Kubernetes CoreDNS
rewrite to `microsandbox-preview-tls-bridge.production.svc.cluster.local`. The
bridge adds PROXY v2 and forwards TLS to the private Cilium Gateway listener.
The route then reaches the production preview proxy, which contacts runner
preview ports over the vSwitch.

The following internal hops use plaintext HTTP:

- API and worker Pods call the Microsandbox control ClusterIP on port `8080`.
- Runner agents call the Microsandbox control NodePort `32080`.
- The control plane calls runner APIs at `10.80.1.2:8081` and
  `10.80.1.3:8081`.
- Cilium forwards terminated HTTP to API, web, Nango, Grafana, and preview
  services over the Pod network.
- The preview proxy calls the preview cache and sandbox preview ports over
  HTTP.

These connections trust the Pod network or private vSwitch and their firewall
boundaries. There is no service mesh or workload mTLS. Compromise of a K3s node,
a runner host, or another machine admitted to the vSwitch can therefore expose
or alter traffic on these HTTP hops. Don't attach unrelated hosts to this
network, and don't treat the vSwitch as equivalent to an authenticated service
identity.

## Network policy coverage

Cilium runs with policy enforcement mode `default`; a Pod becomes ingress or
egress isolated when a matching policy selects it.

- API ingress accepts Cilium ingress, host, and remote-node identities plus
  same-environment Pod traffic on ports `8080` and `8081`.
- Worker Pods deny all ingress. Workers have no Service.
- Web ingress accepts only Cilium ingress, host, and remote-node identities on
  `8080`.
- Nango accepts Gateway traffic and backend Pods on `3003`.
- PostgreSQL accepts Pods in its own namespace and CloudNativePG controller
  reconciliation on port `8000`.
- Redis accepts same-namespace Pods and the Redis operator on Redis data and,
  for production, cluster-bus ports.
- Qdrant allows peer traffic, backend gRPC on `6334`, backup HTTP on `6333`, DNS,
  and outbound HTTPS for S3 snapshots.
- The production preview TLS bridge accepts API and worker traffic from both
  application namespaces; its health port accepts host and remote-node probes.
- Grafana accepts the ingress identity, node identities, and observability
  namespace Pods on port `3000`.

NetworkPolicy isn't a blanket namespace firewall. Several workloads have no
egress policy, and a same-namespace source may have broad access to the selected
PostgreSQL Pods. Review policy selectors whenever labels or service topology
change. Adding a new public HTTPRoute without a matching backend ingress rule
will produce an accepted route that still can't reach its Pod.

## Container and storage controls

Application and infrastructure Pods generally run as non-root, drop Linux
capabilities, disable privilege escalation, use `RuntimeDefault` seccomp, and
mount a read-only root filesystem. Writable paths use bounded `emptyDir`
volumes or Longhorn PVCs. The preview Caddy adds only `NET_BIND_SERVICE`.

Longhorn stores two replicas for newly provisioned volumes, one on each current
node. This protects against one disk or node loss, but it doesn't replace a
database-native backup. PostgreSQL uses Barman base backups plus WAL, Redis
exports validated RDB files, Qdrant creates collection snapshots, and K3s takes
etcd snapshots. Object storage credentials stay namespace- and service-specific.

Production PostgreSQL and Nango PostgreSQL use three database instances even
though the Kubernetes cluster currently has two physical nodes. Staging uses a
single instance to save resources. Microsandbox PostgreSQL also uses one
instance. Replica counts don't create a third failure domain on two machines.

## Adding nodes or runners safely

For a new K3s ingress node, add it to both `k3s_servers` and `k3s_ingress` in
`ansible/inventory/hosts.yml`, then rerun the K3s and runner HAProxy playbooks.
The group controls the Cilium ingress label and every runner HAProxy backend.

For a new runner, add its private address to the Ansible inventory, then also
update `k3s_private_registry_clients` and
`k3s_microsandbox_control_clients`. Apply the K3s firewall configuration before
starting the runner. The runner UFW rules accept API and preview traffic from
the full `10.80.1.0/24` K3s subnet, so future Kubernetes nodes inside that
subnet don't need per-node runner rules.

After either change, verify DNS, HAProxy health, host firewalls, NetworkPolicy,
certificate trust, and an end-to-end sandbox request. A ping test alone proves
almost nothing about these controls.
