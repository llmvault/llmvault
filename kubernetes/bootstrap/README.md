# Production Kubernetes bootstrap

K3s is installed by `ansible/playbooks/k3s/install.yml`. Everything after the
K3s API is available is applied directly with `kubectl`; there is no in-cluster
GitOps controller.

## Pinned versions

- K3s: `v1.35.6+k3s1` (Kubernetes 1.35)
- Gateway API CRDs: `v1.4.1`
- Cilium and Cilium Envoy: `v1.19.6`
- cert-manager: `v1.21.0`

Kubernetes 1.35 is deliberate: it is the newest Kubernetes minor guaranteed by
the pinned stable Cilium release.

## Cluster access

The API server is bound to the private node address and is not exposed on the
public interface. Start an SSH tunnel from the repository root:

```sh
ssh -i ~/.ssh/usehivy -N \
  -L 16443:127.0.0.1:6443 \
  root@95.216.118.156
```

The Ansible playbook exports credentials under the git-ignored
`ansible/.secrets/k8s0/` directory. Create a local-use copy once and point it at
the tunnel:

```sh
cp ansible/.secrets/k8s0/kubeconfig.yaml \
  ansible/.secrets/k8s0/kubeconfig-local.yaml

kubectl --kubeconfig ansible/.secrets/k8s0/kubeconfig-local.yaml \
  config set-cluster default --server=https://127.0.0.1:16443

export KUBECONFIG="$PWD/ansible/.secrets/k8s0/kubeconfig-local.yaml"
```

Treat every file below `ansible/.secrets/` as production recovery material.
Keep file mode `0600` and store an encrypted copy outside the repository.

## Gateway API CRDs

Cilium 1.19 is tested with Gateway API 1.4. Apply the standard CRDs before
rendering Cilium so its operator starts with the required APIs available:

```sh
kubectl apply --server-side=true \
  -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/standard-install.yaml
```

## Cilium

Render the pinned chart into the git-ignored secrets directory. The committed
values file is the source of truth; Helm does not create or manage a release.

```sh
helm template cilium oci://quay.io/cilium/charts/cilium \
  --version 1.19.6 \
  --namespace kube-system \
  --values kubernetes/bootstrap/cilium/values.yaml \
  > ansible/.secrets/k8s0/cilium-v1.19.6.yaml

kubectl apply --server-side=true \
  --field-manager=hivy-bootstrap \
  -f ansible/.secrets/k8s0/cilium-v1.19.6.yaml
```

Validate before installing anything else:

```sh
kubectl -n kube-system rollout status daemonset/cilium --timeout=5m
kubectl -n kube-system rollout status daemonset/cilium-envoy --timeout=5m
kubectl -n kube-system rollout status deployment/cilium-operator --timeout=5m
kubectl get nodes -o wide
```

## cert-manager

cert-manager is applied from its pinned upstream static manifest:

```sh
kubectl apply --server-side=true \
  -f https://github.com/cert-manager/cert-manager/releases/download/v1.21.0/cert-manager.yaml

kubectl -n cert-manager wait --for=condition=Available deployment \
  --all --timeout=5m
```

Certificate issuers and DNS credentials are separate manifests and secrets;
they are not embedded in this bootstrap document.

## Hetzner public load balancer

The public edge uses TCP pass-through so Cilium Envoy owns HTTP routing, TLS
termination, and certificates. The current smallest Hetzner load balancer is:

- name: `k8s-public-ingress`
- resource ID: `7249812`
- type and location: `lb11`, `hel1`
- public IPv4: `65.109.40.68`
- public IPv6: `2a01:4f9:c01d:2cf::1` (allocated, but do not publish)
- private IPv4: `10.80.0.4`
- target: bare-metal node `10.80.1.4`

Its services are intentionally simple:

| Public listener | Private target | Protocol | PROXY protocol |
| --- | --- | --- | --- |
| `80` | `10080` | TCP | enabled |
| `443` | `10443` | TCP | enabled |

Both services use TCP health checks against their target port. The host
firewall permits those target ports only from `10.80.0.4`. Cilium is configured
to consume PROXY protocol, so Envoy preserves the original client address in
`X-Forwarded-For` while TLS remains end-to-end from the client to Envoy.

Do not configure TLS certificates on the Hetzner load balancer. Do not point
production DNS at its public addresses until the application migration is
ready for cutover.

### IPv6 cutover constraint

Publish an IPv4 `A` record only at the initial DNS cutover. An external IPv6
probe reaches Envoy but currently returns `500` when Hetzner TCP forwarding and
PROXY protocol are combined with Cilium Gateway API. This matches the upstream
Cilium issue `#42950`, which remains open. Do not publish an `AAAA` record until
that issue is fixed in a tested Cilium release or the edge design changes. IPv4
PROXY protocol is healthy and preserves the original client IP.

## Private application routing

The same Cilium Gateway owns a private HTTPS listener on port `443`. Kubernetes
CoreDNS rewrites `*.preview.usehivy.com` to the Gateway Service ClusterIP, so
API and asynq Pods reach the in-cluster preview Caddy without leaving the
cluster. Runner-local CoreDNS resolves `api.usehivy.com` and
`staging.api.usehivy.com` to that runner's private HAProxy listener. HAProxy
passes TLS through to a healthy private K3s ingress node. This path does not
use the Hetzner load balancer.

## Gateway smoke test

The smoke stack proves HTTP redirect, HTTPS termination, HTTP/2, Gateway API
routing, and client-IP preservation without changing DNS. It uses a temporary
self-signed certificate and a Kubernetes `agnhost` echo workload:

```sh
kubectl apply -k kubernetes/smoke/gateway
kubectl wait -n gateway-smoke --for=condition=Available \
  deployment/gateway-echo --timeout=5m

curl --resolve gateway-check.usehivy.com:80:65.109.40.68 \
  -D - -o /dev/null http://gateway-check.usehivy.com/

curl --insecure --http2 \
  --resolve gateway-check.usehivy.com:443:65.109.40.68 \
  https://gateway-check.usehivy.com/hostname

curl --insecure --http2 \
  --resolve gateway-check.usehivy.com:443:65.109.40.68 \
  https://gateway-check.usehivy.com/header
```

The first request must return `301`, the second must return an echo pod name,
and the final JSON response must contain the caller's public IP in
`X-Forwarded-For`.

After any new Secrets are created, refresh the git-ignored operator export:

```sh
kubectl get secrets --all-namespaces -o yaml \
  > ansible/.secrets/k8s0/cluster-secrets.yaml
chmod 600 ansible/.secrets/k8s0/*
```

## Ordinary manifests

Platform and application manifests live under `kubernetes/` as Kustomize
bases and production overlays. Render before applying:

```sh
kubectl kustomize kubernetes/<component>/overlays/production
kubectl apply -k kubernetes/<component>/overlays/production
```

Do not commit rendered Secrets, kubeconfigs, tokens, or generated bootstrap
manifests. Store them only beneath `ansible/.secrets/`.

The post-bootstrap storage and data operators are documented in
[`../operators/README.md`](../operators/README.md).
