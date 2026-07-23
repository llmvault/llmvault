# Cluster bootstrap

The cluster runs K3s servers with embedded etcd. Every current server also runs
workloads. Ansible creates the host and K3s layer; after the Kubernetes API
answers, operators apply cluster components directly with `kubectl`.

Pinned versions in the repository:

| Component | Version |
| --- | --- |
| K3s | `v1.35.6+k3s1` |
| Kubernetes Gateway API standard CRDs | `v1.4.1` |
| Cilium and Cilium Envoy | `v1.19.6` |
| cert-manager | `v1.21.0` |
| Vercel cert-manager webhook | `v1.0.0` image digest from the committed values file |

K3s disables Flannel, kube-proxy, Traefik, ServiceLB, the bundled network-policy
controller, and local-path storage. A newly installed node stays `NotReady`
until Cilium runs; that is expected during the first bootstrap.

## Before running Ansible

The current servers run Ubuntu 26.04 amd64; the role accepts Ubuntu 24 or newer.
Prepare the host with swap removed, SSH root access, and its public interface
available as `enp0s31f6`. Attach the host to Hetzner vSwitch VLAN 4000 before
the playbook. The K3s role creates the VLAN Netplan and address, but it cannot
attach a physical server to the provider network.

On the operator machine:

```sh
cd ansible
ansible-galaxy collection install -r requirements.yml
ansible k8s_servers -m ping
```

Check these inventory settings before a fresh install:

- a unique `k3s_node_name`, private `k3s_node_ip`, and VLAN interface for every
  host;
- `k3s_bootstrap_host: k8s0`;
- identical Pod, Service, private-network, reservation, and etcd settings for
  every server;
- the exact K3s release checksum from the upstream release;
- every intended Gateway node listed in `k3s_ingress`.

Live kubeconfigs, K3s tokens, rendered charts, and provider credentials belong
under the ignored `kubernetes/config/` directory. Run
`kubernetes/config/validate.sh` after restoring the operator backup.

## Install the first K3s server

Install `k8s0` alone first. It generates the shared server token, starts the
embedded-etcd cluster, and exports the token that later servers need.

```sh
cd ansible
ansible-playbook playbooks/k3s/install.yml --limit k8s0
ansible-playbook playbooks/k3s/validate.yml --limit k8s0
```

The visible K3s configuration lands at `/etc/rancher/k3s/config.yaml`. The API
binds to `10.80.1.4:6443`; it does not listen on the public node address. K3s
encrypts Kubernetes Secret values at rest and reserves these resources from
ordinary Pod scheduling on each node:

- system: `1000m` CPU, `2Gi` memory, `5Gi` ephemeral storage;
- Kubernetes: `1000m` CPU, `2Gi` memory, `5Gi` ephemeral storage.

Kubelet starts eviction at less than `2Gi` available memory, less than 10%
available node filesystem, or less than 15% available image filesystem.

K3s schedules an embedded-etcd snapshot every six hours and retains 14. Its
config expects Secret `kube-system/k3s-etcd-s3`. The ignored recovery file is
`kubernetes/config/credentials/k3s/k8s0/k3s-etcd-s3.yaml`. Apply it through the
administrator tunnel below, then confirm a manual snapshot can reach object
storage.

## Open an administrator tunnel

Use SSH because the Kubernetes API is private. The administrator kubeconfig
exported by Ansible points at a private address, so make a local copy for the
tunnel:

```sh
ssh -i ~/.ssh/usehivy -N \
  -L 16443:127.0.0.1:6443 \
  root@95.216.118.156
```

In another terminal, from the repository root:

```sh
cp kubernetes/config/kubeconfigs/k8s0/admin.yaml \
  kubernetes/config/kubeconfigs/k8s0/local.yaml

kubectl --kubeconfig kubernetes/config/kubeconfigs/k8s0/local.yaml \
  config set-cluster default --server=https://127.0.0.1:16443

export KUBECONFIG="$PWD/kubernetes/config/kubeconfigs/k8s0/local.yaml"
kubectl get --raw=/readyz

kubectl apply -f kubernetes/config/credentials/k3s/k8s0/k3s-etcd-s3.yaml
ssh -i ~/.ssh/usehivy root@95.216.118.156 \
  'k3s etcd-snapshot save --name bootstrap-check'
ssh -i ~/.ssh/usehivy root@95.216.118.156 'k3s etcd-snapshot list'
```

Do not commit `local.yaml`; it contains administrator credentials.

## Install Gateway API CRDs

Cilium needs Gateway API types present before its operator starts:

```sh
kubectl apply --server-side=true \
  -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/standard-install.yaml

kubectl api-resources --api-group=gateway.networking.k8s.io
```

The repository uses standard CRDs, not the experimental bundle.

## Render and install Cilium

The committed values file controls the Cilium install. Helm only renders a
manifest; no Helm release owns Cilium.

```sh
mkdir -p kubernetes/config/generated/k8s0

helm template cilium oci://quay.io/cilium/charts/cilium \
  --version 1.19.6 \
  --namespace kube-system \
  --values kubernetes/bootstrap/cilium/values.yaml \
  > kubernetes/config/generated/k8s0/cilium-v1.19.6.yaml

kubectl apply --server-side=true \
  --field-manager=hivy-bootstrap \
  -f kubernetes/config/generated/k8s0/cilium-v1.19.6.yaml
```

The chosen Cilium setup replaces kube-proxy, allocates Pod addresses from
`10.42.0.0/16`, tunnels Pod traffic over VXLAN, enables policy enforcement,
runs Envoy, and creates the GatewayClass. Cilium talks to K3s through each
node's local supervisor at `127.0.0.1:6444`, which avoids making one server's IP
a cluster-wide CNI dependency.

Gateway listeners run on the host network only on nodes labelled
`hivy.io/public-ingress=true`. PROXY protocol is mandatory because both the
Hetzner Load Balancer and runner HAProxy send PROXY v2.

Wait for networking before installing other Pods:

```sh
kubectl -n kube-system rollout status daemonset/cilium --timeout=5m
kubectl -n kube-system rollout status daemonset/cilium-envoy --timeout=5m
kubectl -n kube-system rollout status deployment/cilium-operator --timeout=5m
kubectl get nodes -o wide
kubectl get gatewayclass cilium
```

If a node stays `NotReady`, start with `kubectl -n kube-system get pods -o wide`
and `journalctl -u k3s` on that node. Do not install Flannel or re-enable
kube-proxy as a workaround; that creates two competing network stacks.

## Join more K3s servers

After `k8s0` has exported its server token, install the next inventory host:

```sh
cd ansible
ansible-playbook playbooks/k3s/install.yml --limit k8s1
ansible-playbook playbooks/k3s/validate.yml --limit k8s1
```

Reconcile the existing servers afterward so their shared TLS SAN list matches
the expanded inventory. The playbook's `serial: 1` guard reconciles one server
at a time:

```sh
ansible-playbook playbooks/k3s/install.yml
ansible-playbook playbooks/k3s/validate.yml
```

Then check membership and Cilium coverage:

```sh
kubectl get nodes -o wide
kubectl -n kube-system get pods -o wide -l k8s-app=cilium
kubectl -n kube-system get pods -o wide -l k8s-app=cilium-envoy
ssh -i ~/.ssh/usehivy root@95.216.118.156 'k3s etcd-snapshot list'
```

The current two-member etcd cluster requires both members for quorum. Add a
third server before calling the control plane highly available. Workload
replicas can survive one worker loss only where their placement, storage, and
disruption rules allow it.

For an ingress node, add its private address to both Hetzner Load Balancer TCP
services and reconcile runner HAProxy after Cilium and Envoy report healthy:

```sh
cd ansible
ansible-playbook playbooks/runner-haproxy.yml
```

## Install cert-manager and the Vercel solver

Install the pinned upstream cert-manager manifest:

```sh
kubectl apply --server-side=true \
  -f https://github.com/cert-manager/cert-manager/releases/download/v1.21.0/cert-manager.yaml

kubectl -n cert-manager wait --for=condition=Available deployment \
  --all --timeout=5m
```

Create the ignored Vercel credential Secret in `cert-manager`. Ensure the token
file contains only the token and has no trailing newline before using
`--from-file`:

```sh
kubectl -n cert-manager create secret generic vercel-credentials \
  --from-file=token=kubernetes/config/credentials/providers/vercel-token \
  --dry-run=client -o yaml | kubectl apply -f -
```

Render and apply the Vercel webhook. The current repository expects the
`v1.0.0` chart archive at `/tmp/cert-manager-webhook-vercel-v1.0.0.tgz`; it does
not contain a download or checksum command for that archive.

```sh
mkdir -p kubernetes/config/generated/k8s0/operators

helm template cert-manager-webhook-vercel \
  /tmp/cert-manager-webhook-vercel-v1.0.0.tgz \
  --namespace cert-manager \
  --values kubernetes/bootstrap/cert-manager/vercel-webhook-values.yaml \
  > kubernetes/config/generated/k8s0/operators/cert-manager-webhook-vercel-v1.0.0.yaml

kubectl apply \
  -f kubernetes/config/generated/k8s0/operators/cert-manager-webhook-vercel-v1.0.0.yaml
kubectl apply -f kubernetes/bootstrap/cert-manager/vercel-cluster-issuer.yaml
```

Verify the webhook and issuer before requesting the shared certificate:

```sh
kubectl -n cert-manager get pods
kubectl get clusterissuer letsencrypt-production-vercel
kubectl describe clusterissuer letsencrypt-production-vercel
```

## Apply the Gateway and shared certificate

No manifest in `kubernetes/ingress/public` creates its namespace. Create
`ingress-public` explicitly on a fresh cluster:

```sh
kubectl create namespace ingress-public --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -k kubernetes/ingress/public
```

The package creates one Gateway with public HTTP and HTTPS listeners plus a
private HTTPS listener. It also adds the Kubernetes CoreDNS wildcard rewrite
for private preview traffic and requests the shared public certificate.

Wait for the certificate and route controller:

```sh
kubectl -n ingress-public wait --for=condition=Ready \
  certificate/preview-usehivy --timeout=10m
kubectl -n ingress-public get gateway public
kubectl -n ingress-public describe gateway public
kubectl -n kube-system rollout status deployment/coredns --timeout=5m
```

At this point, configure the Hetzner Load Balancer outside Kubernetes:

| Listener | Target | Mode |
| --- | --- | --- |
| TCP `80` | every ingress node on `10080` | PROXY protocol enabled, TCP check |
| TCP `443` | every ingress node on `10443` | PROXY protocol enabled, TCP check |

Attach the Load Balancer to the private network as `10.80.0.4`. Do not install a
certificate on it; Envoy owns TLS. Publish application IPv4 `A` records to
`65.109.40.68` only after the matching HTTPRoutes and Services exist. Leave
`AAAA` records absent under the current edge setup.

## Bootstrap validation

Check node listeners from the Load Balancer side or another permitted private
host, then exercise the public address without changing DNS:

```sh
curl --resolve api.usehivy.com:80:65.109.40.68 \
  -D - -o /dev/null http://api.usehivy.com/

curl --resolve api.usehivy.com:443:65.109.40.68 \
  -D - -o /dev/null https://api.usehivy.com/healthz
```

The HTTP response should redirect to HTTPS. The HTTPS certificate must verify,
and its response should come from the backend route once that workload exists.

The `kubernetes/smoke/gateway` package adds an echo workload and routes for
`gateway-check.usehivy.com`, but its self-signed Secret is not referenced by the
current shared Gateway. `curl --insecure` can test routing, HTTP/2, and forwarded
headers; it does not prove that `gateway-smoke-tls` is attached. Remove the
smoke namespace after testing:

```sh
kubectl apply -k kubernetes/smoke/gateway
kubectl wait -n gateway-smoke --for=condition=Available \
  deployment/gateway-echo --timeout=5m
curl --insecure --http2 \
  --resolve gateway-check.usehivy.com:443:65.109.40.68 \
  https://gateway-check.usehivy.com/header
kubectl delete -k kubernetes/smoke/gateway
```

## What comes after bootstrap

Install Longhorn host prerequisites and the Longhorn, CloudNativePG, Barman
Cloud, and Redis controllers next. Apply their committed or pinned rendered
manifests with `kubectl`; Ansible does not install them. Application namespaces,
data clusters, observability, Gateway routes, and workloads follow after those
controllers report ready.

After creating or rotating Kubernetes Secrets, refresh the ignored recovery
export without printing Secret payloads:

```sh
umask 077
kubectl get secrets --all-namespaces -o yaml \
  > kubernetes/config/credentials/k3s/k8s0/cluster-secrets.yaml
chmod 600 kubernetes/config/credentials/k3s/k8s0/cluster-secrets.yaml
```
