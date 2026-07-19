# Networking

This file describes the routes declared in this repository. DNS records and the
Hetzner Load Balancer live outside Kubernetes, so compare their live state with
this file during an incident.

## Address plan

| Purpose | Range or address | Where it comes from |
| --- | --- | --- |
| Hetzner private network | `10.80.0.0/16` | `ansible/inventory/group_vars/k3s_servers.yml` |
| Bare-metal vSwitch segment | `10.80.1.0/24` | Hetzner vSwitch VLAN 4000 |
| vSwitch gateway | `10.80.1.1` | K3s Netplan template |
| Runner 0 | public `135.181.238.109`, private `10.80.1.2` | Ansible inventory |
| Runner 1 | public `157.180.98.55`, private `10.80.1.3` | Ansible inventory |
| K3s node 0 | public `95.216.118.156`, private `10.80.1.4` | Ansible inventory |
| K3s node 1 | public `95.216.224.189`, private `10.80.1.5` | Ansible inventory |
| Hetzner Load Balancer | public `65.109.40.68`, private `10.80.0.4` | manually managed in Hetzner |
| Pod addresses | `10.42.0.0/16`, one `/24` pool per node | Cilium cluster-pool IPAM |
| Service addresses | `10.43.0.0/16` | K3s configuration |
| Sandbox guest addresses accepted by runner DNS and HTTPS | `172.16.0.0/12` | runner CoreDNS and HAProxy roles |

The K3s nodes attach `enp0s31f6.4000` to VLAN 4000 with MTU 1400. Cilium uses
VXLAN with MTU 1350. The runner private-interface setup is not declared in the
current Ansible roles; prepare that interface before running the runner
playbooks.

## Public request path

Public `A` records point at `65.109.40.68`. Do not publish the allocated Load
Balancer IPv6 address: the current Cilium and Hetzner PROXY protocol combination
returns `500` over IPv6.

```text
client
  -> Vercel-hosted DNS A record
  -> Hetzner Load Balancer 65.109.40.68
  -> 10.80.1.4 or 10.80.1.5 over the private network
  -> Cilium Envoy host-network listener
  -> Gateway API HTTPRoute
  -> ClusterIP Service
  -> application Pod
```

The Load Balancer runs TCP pass-through, not HTTP termination:

| Public port | Node port | Transport | PROXY v2 |
| --- | --- | --- | --- |
| `80` | `10080` | TCP | yes |
| `443` | `10443` | TCP | yes |

Cilium Envoy terminates TLS with `ingress-public/preview-usehivy-tls`. The
certificate comes from cert-manager through a Vercel DNS-01 solver. Port 80
routes redirect HTTP to HTTPS. The node firewall accepts ports `10080` and
`10443` only from Load Balancer address `10.80.0.4`.

The public Gateway currently routes these names:

| Name | Destination |
| --- | --- |
| `usehivy.com` and `staging.usehivy.com` | environment web Service |
| `api.usehivy.com` and `staging.api.usehivy.com` | environment backend API on `8080` |
| `proxy.usehivy.com` and `staging.proxy.usehivy.com` | backend API after rewriting `/` to `/v1/proxy/` |
| `connections.usehivy.com` and `staging.connections.usehivy.com` | environment Nango Service on `3003` |
| `mcp.usehivy.com` and `staging.mcp.usehivy.com` | environment backend MCP port `8081` |
| `preview.usehivy.com` and `*.preview.usehivy.com` | production Microsandbox preview proxy |
| `monitor.usehivy.com` | Grafana |

The shared certificate covers both MCP names. cert-manager creates DNS-01
challenge records through the webhook, but it does not create the application
`A` records. An operator still changes those records during a cutover.

Vercel hosts the authoritative DNS zone. cert-manager creates DNS-01 challenge
records through the webhook, but it does not create the application `A`
records. An operator still changes those records during a cutover.

## Service-to-service traffic inside Kubernetes

Pods should use Kubernetes Service names when the caller does not need a
browser-visible URL. Current examples include:

| Caller | Destination |
| --- | --- |
| production web | `http://backend-api.production.svc.cluster.local:8080` |
| staging web | `http://backend-api.staging.svc.cluster.local:8080` |
| backend in either namespace | namespace-local `http://nango:3003` |
| backend in either namespace | `http://microsandbox-control.production.svc.cluster.local:8080` |
| backend | namespace-local PostgreSQL, Redis, and Qdrant Services |
| preview cache | `http://microsandbox-control:8080` in production |

These links do not pass through the public Load Balancer. Most use plain HTTP
or native database protocols after the request enters the cluster. Cilium
network policy limits many ingress paths, but the cluster does not enable
WireGuard, IPsec, or service-to-service mTLS. The Hetzner vSwitch and Cilium
policy provide isolation; they do not encrypt cleartext application traffic.

## Egress and callback map

| Source | Destination | Path and transport |
| --- | --- | --- |
| Browser | web, API, Nango, previews, Grafana | Public DNS to the Hetzner Load Balancer; HTTPS ends at Cilium Envoy |
| Next.js server | backend API | Namespace-local Service over HTTP |
| API and worker | PostgreSQL, Redis, Qdrant, Nango | Namespace-local Services over PostgreSQL, Redis, gRPC, and HTTP |
| API and worker | Microsandbox control | Production ClusterIP Service over HTTP, including calls from staging |
| Sandbox guest | API, LLM proxy, and MCP | Runner split DNS and runner HAProxy; HTTPS stays on the vSwitch and ends at Envoy |
| API and worker | sandbox preview URL | Cluster DNS rewrite and preview TLS bridge; HTTPS ends at Envoy, then HTTP crosses the private runner path |
| Microsandbox control | runner agent | Runner private address on HTTP `8081` |
| Runner agent | Microsandbox control | Fixed private NodePort URL on HTTP `32080` |
| Runner and BuildKit | Zot | Runner-local HAProxy to private NodePort; Zot terminates TLS |
| API | Hetzner Object Storage, Resend, Paystack, model providers, OAuth providers | Public provider endpoints, normally HTTPS |
| Backup jobs and controllers | Hetzner Object Storage | Public S3-compatible endpoint over HTTPS |
| Nango | OAuth and integration providers | Public provider endpoints over HTTPS |
| cert-manager | Let's Encrypt and Vercel DNS API | Public endpoints over HTTPS |
| GitHub Actions | Kubernetes API | SSH to a K3s public address, then a restricted tunnel to `127.0.0.1:6443` |

The backend reaches Nango privately at `http://nango:3003`. OAuth browser flows
still require the public connections hostname. The Kubernetes manifests do not
declare Nango's outbound webhook destination; that setting can live in Nango's
database. Verify the saved target after a Nango database migration instead of
assuming that the reverse Nango-to-API call uses the cluster Service.

## Runner and sandbox private HTTPS

Every runner runs its own CoreDNS and HAProxy. The Microsandbox runner passes
its private address as the DNS server for sandbox guests.

Runner CoreDNS maps these names to that same runner's private address:

- `api.usehivy.com`
- `staging.api.usehivy.com`
- `proxy.usehivy.com`
- `staging.proxy.usehivy.com`
- `mcp.usehivy.com`
- `staging.mcp.usehivy.com`

All other DNS queries go to the runner host's system resolver. The six private
names then take this path:

```text
sandbox or runner host
  -> runner CoreDNS returns 10.80.1.2 or 10.80.1.3
  -> local HAProxy port 443
  -> round-robin healthy member of Ansible group k3s_ingress, port 443
  -> Cilium private-https Gateway listener
  -> matching HTTPRoute
  -> backend Service
```

HAProxy does not terminate TLS. It prepends PROXY v2 and passes the original TLS
session to Cilium Envoy, where the public certificate terminates it. Sandbox
traffic to the API, model proxy, and MCP endpoints therefore stays on the
private network without giving up HTTPS hostname verification.

Runner UFW permits guest addresses in `172.16.0.0/12` to query the runner's TCP
and UDP port 53 and to reach its private port 443. It denies other inbound
traffic unless another role adds a rule.

The MCP hostnames use the same split-DNS and TCP pass-through path as API and
proxy traffic. This is required because the public load balancer expects PROXY
v2; sandbox TLS sent directly to it is rejected during the handshake.

## Preview URLs

Browsers and Kubernetes Pods use the same `https://<id>.preview.usehivy.com`
URL, but split-horizon DNS changes the first hop.

A browser follows the public request path to the Gateway. Envoy terminates TLS,
then sends HTTP to the in-cluster Caddy deployment
`microsandbox-preview-proxy`. Caddy asks `microsandbox-preview-cache` to resolve
the preview hostname, then proxies to the selected runner's private host port.
Runner UFW accepts preview ports `30000-60999` only from `10.80.1.0/24`.

Kubernetes CoreDNS rewrites the wildcard preview name to
`microsandbox-preview-tls-bridge.production.svc.cluster.local`. That bridge is
an in-cluster HAProxy deployment. It accepts ordinary TLS on Service port 443,
prepends PROXY v2, and forwards to
`cilium-gateway-public.ingress-public.svc.cluster.local:443`. The rest of the
route matches the browser path. This bridge exists because Cilium's Gateway
listeners expect PROXY v2; an ordinary Pod TLS connection sent straight to the
private listener would fail before HTTP routing.

```text
backend or asynq Pod
  -> Kubernetes CoreDNS wildcard rewrite
  -> preview TLS bridge
  -> Cilium private-https listener
  -> preview Caddy
  -> preview cache lookup
  -> runner 10.80.1.2 or 10.80.1.3, port 30000-60999
```

TLS ends at Envoy. Caddy's final connection to the runner preview port uses HTTP
on the private vSwitch.

## Microsandbox control traffic

The backend and preview cache call the control plane through its production
ClusterIP Service. The control plane stores each runner's private HTTP URL and
calls the runner on `10.80.1.2:8081` or `10.80.1.3:8081`. UFW permits port 8081
from the complete Kubernetes segment `10.80.1.0/24`, and a Cilium policy admits
the runner addresses at the control-plane Pods.

Runners currently call the control plane at `http://10.80.1.4:32080`. That
NodePort uses `externalTrafficPolicy: Local`. The configured URL therefore has
a single-node dependency even though the control deployment has two replicas;
adding a K3s node does not update the URL automatically.

The control and runner APIs use cleartext HTTP on the private network. Tokens
authenticate both directions. There is no transport encryption on these two
links.

## Private Zot registry

Zot runs in the production namespace and terminates TLS itself. It has no
Gateway route and the Hetzner Load Balancer has no registry listener.

```text
runner resolves registry.usehivy.com through /etc/hosts
  -> that runner's private address, port 5000
  -> runner HAProxy
  -> healthy K3s node, NodePort 32500
  -> Zot Pod, port 5000
```

Zot's NodePort uses `externalTrafficPolicy: Local`, so only a node with the Zot
Pod answers the HAProxy check. The node firewall and Cilium policy list the two
current runner private addresses. Add a future runner to both controls before
letting it pull or push images.

## Host firewall ports

K3s node nftables defaults to drop inbound traffic. It accepts:

| Source | Destination port or protocol | Reason |
| --- | --- | --- |
| any | TCP `22` | SSH administration and deployment tunnels |
| Pod and Service CIDRs | any host input | traffic already processed by Cilium |
| `10.80.0.0/16` on the vSwitch | TCP `2379`, `2380`, `4240`, `4244`, `4245`, `6443`, `10250`; UDP `8472` | etcd, Kubernetes, Cilium, and VXLAN |
| `10.80.0.4` | TCP `10080`, `10443` | public Load Balancer targets |
| `10.80.0.0/16` | TCP `443` | private Cilium Gateway |
| listed runners | TCP `32080`, `32500` | Microsandbox control and Zot NodePorts |

Runner UFW defaults to deny inbound and allow outbound. It accepts SSH from any
source, runner API `8081` and preview ports `30000-60999` from
`10.80.1.0/24`, guest DNS from `172.16.0.0/12`, guest HTTPS to its own private
address, and registry port `5000` from its own private address.

## Network checks

Check Gateway and route admission:

```sh
kubectl get gateway -n ingress-public public
kubectl get httproute -A
kubectl describe gateway -n ingress-public public
```

Check public routing without waiting for local DNS caches:

```sh
curl --resolve api.usehivy.com:443:65.109.40.68 https://api.usehivy.com/healthz
curl --resolve staging.api.usehivy.com:443:65.109.40.68 https://staging.api.usehivy.com/healthz
openssl s_client -connect 65.109.40.68:443 -servername staging.mcp.usehivy.com -verify_return_error -brief </dev/null
```

Check a runner's private resolution and HAProxy pool:

```sh
ssh -i ~/.ssh/usehivy root@135.181.238.109 \
  'resolvectl query api.usehivy.com; getent ahostsv4 proxy.usehivy.com; getent ahostsv4 staging.mcp.usehivy.com'

ssh -i ~/.ssh/usehivy root@135.181.238.109 \
  'printf "show stat\n" | socat - UNIX-CONNECT:/run/haproxy/admin.sock'
```

Test the private TLS path on a runner without changing DNS:

```sh
ssh -i ~/.ssh/usehivy root@135.181.238.109 \
  'curl --resolve api.usehivy.com:443:10.80.1.2 https://api.usehivy.com/healthz'

ssh -i ~/.ssh/usehivy root@135.181.238.109 \
  'openssl s_client -connect 10.80.1.2:443 -servername staging.mcp.usehivy.com -verify_return_error -brief </dev/null'
```

Check vSwitch reachability from a K3s node:

```sh
ssh -i ~/.ssh/usehivy root@95.216.118.156 \
  'ping -c 2 10.80.1.2; curl --fail http://10.80.1.2:8081/health'
```

For preview routing, compare DNS inside a backend Pod with public DNS. The Pod
answer should point at the preview TLS bridge Service rather than the public
Load Balancer:

```sh
kubectl exec -n staging deploy/backend-api -- getent ahostsv4 test.preview.usehivy.com
kubectl -n production get pods,svc -l app.kubernetes.io/name=microsandbox-preview-tls-bridge
kubectl -n production logs deploy/microsandbox-preview-proxy --since=10m
```
