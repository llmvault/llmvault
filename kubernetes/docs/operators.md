# Storage and data operators

Longhorn, CloudNativePG, the Barman Cloud plugin, and the OT Redis Operator are
cluster-wide controllers. Ansible does not install them. Operators download or
render pinned artifacts into the ignored `kubernetes/config/generated/` tree,
then apply them directly with `kubectl`.

| Controller | Version | Namespace | Ownership |
| --- | --- | --- | --- |
| Longhorn | `v1.12.0` | `longhorn-system` | Helm-rendered YAML, direct apply |
| CloudNativePG | `v1.30.0` | `cnpg-system` | Pinned upstream YAML, direct apply |
| Barman Cloud plugin | `v0.13.0` | `cnpg-system` | Pinned upstream YAML, direct apply |
| OT Redis Operator | `v0.26.0` | `redis-operator` | Helm-rendered YAML, direct apply |

Apply the namespaces first:

```sh
kubectl apply -f kubernetes/operators/namespaces.yaml
```

## Longhorn host preparation

The K3s Ansible role does not currently install Longhorn's host dependencies.
Every storage node needs `iscsid`, NFS support, `iscsi_tcp`, `nfs`, and
`dm_crypt`. The unused multipath daemon must remain disabled so it cannot claim
Longhorn devices.

Run the pinned Longhorn preflight installer and check from the operator
machine:

```sh
longhornctl --kubeconfig "$KUBECONFIG" \
  --image longhornio/longhorn-cli:v1.12.0 install preflight

longhornctl --kubeconfig "$KUBECONFIG" check preflight
```

Persist the repository's module list and service state on every storage node:

```sh
for host in 95.216.118.156 95.216.224.189; do
  scp -i ~/.ssh/usehivy \
    kubernetes/operators/longhorn/host-modules.conf \
    "root@$host:/etc/modules-load.d/longhorn.conf"

  ssh -i ~/.ssh/usehivy "root@$host" \
    'modprobe nfs && modprobe dm_crypt && modprobe iscsi_tcp && \
     systemctl enable --now iscsid && \
     systemctl disable --now multipathd.service multipathd.socket && \
     systemctl mask multipathd.service multipathd.socket'
done
```

This manual step must be repeated for each new storage node. A future Ansible
role can own it, but the current inventory does not.

## Longhorn

Download, verify, render, and apply the chart:

```sh
mkdir -p kubernetes/config/generated/k8s0/operators

curl -fsSL \
  https://github.com/longhorn/charts/releases/download/longhorn-1.12.0/longhorn-1.12.0.tgz \
  -o kubernetes/config/generated/k8s0/operators/longhorn-1.12.0.tgz

printf '%s  %s\n' \
  869bb20701b154473606f1e8967b27f34f2448a2dfe6eb8970f1cae6957384f5 \
  kubernetes/config/generated/k8s0/operators/longhorn-1.12.0.tgz | shasum -a 256 -c -

helm template longhorn \
  kubernetes/config/generated/k8s0/operators/longhorn-1.12.0.tgz \
  --namespace longhorn-system \
  --include-crds \
  --no-hooks \
  --values kubernetes/operators/longhorn/values.yaml \
  > kubernetes/config/generated/k8s0/operators/longhorn-v1.12.0.yaml

kubectl apply --server-side=true --field-manager=hivy-operators \
  -f kubernetes/config/generated/k8s0/operators/longhorn-v1.12.0.yaml
```

## CloudNativePG

```sh
curl -fsSL \
  https://github.com/cloudnative-pg/cloudnative-pg/releases/download/v1.30.0/cnpg-1.30.0.yaml \
  -o kubernetes/config/generated/k8s0/operators/cnpg-v1.30.0.yaml

printf '%s  %s\n' \
  f8bede43fe4ee0d478c2355b204a36876b2ae4faac60f2a9452280b293da3b88 \
  kubernetes/config/generated/k8s0/operators/cnpg-v1.30.0.yaml | shasum -a 256 -c -

kubectl apply --server-side=true --field-manager=hivy-operators \
  -f kubernetes/config/generated/k8s0/operators/cnpg-v1.30.0.yaml
```

## Barman Cloud plugin

Install the plugin after CloudNativePG and before applying a PostgreSQL Cluster
that names it as a WAL archiver:

```sh
curl -fsSL \
  https://github.com/cloudnative-pg/plugin-barman-cloud/releases/download/v0.13.0/manifest.yaml \
  -o kubernetes/config/generated/k8s0/operators/barman-cloud-v0.13.0.yaml

printf '%s  %s\n' \
  d2e71e7b06822448f1a421f05781846cfdb9cc621e7ef32eef5e20c5133213b0 \
  kubernetes/config/generated/k8s0/operators/barman-cloud-v0.13.0.yaml | shasum -a 256 -c -

kubectl apply --server-side=true --field-manager=hivy-operators \
  -f kubernetes/config/generated/k8s0/operators/barman-cloud-v0.13.0.yaml

kubectl -n cnpg-system rollout status deployment/barman-cloud --timeout=5m
```

## OT Redis Operator

The webhook stays disabled in the committed values. The CRD schemas and
controller own the standalone staging Redis and production Redis Cluster.

```sh
curl -fsSL \
  https://github.com/OT-CONTAINER-KIT/redis-operator/archive/refs/tags/v0.26.0.tar.gz \
  -o kubernetes/config/generated/k8s0/operators/redis-operator-v0.26.0.tar.gz

printf '%s  %s\n' \
  0389653cc2235ba1149870be75c5e20391760ee0c9eb71283478ab23171ebfff \
  kubernetes/config/generated/k8s0/operators/redis-operator-v0.26.0.tar.gz | shasum -a 256 -c -

tar -xzf \
  kubernetes/config/generated/k8s0/operators/redis-operator-v0.26.0.tar.gz \
  -C kubernetes/config/generated/k8s0/operators

helm template redis-operator \
  kubernetes/config/generated/k8s0/operators/redis-operator-0.26.0/charts/redis-operator \
  --namespace redis-operator \
  --include-crds \
  --no-hooks \
  --values kubernetes/operators/redis-operator/values.yaml \
  > kubernetes/config/generated/k8s0/operators/redis-operator-v0.26.0.yaml

kubectl apply --server-side=true --field-manager=hivy-operators \
  -f kubernetes/config/generated/k8s0/operators/redis-operator-v0.26.0.yaml
```

## Validation

```sh
kubectl -n longhorn-system get pods
kubectl -n cnpg-system rollout status deployment/cnpg-controller-manager
kubectl -n cnpg-system rollout status deployment/barman-cloud
kubectl -n redis-operator rollout status deployment/redis-operator

kubectl apply -k kubernetes/smoke/storage
kubectl wait -n storage-smoke --for=condition=Ready \
  pod/longhorn-volume-smoke --timeout=5m
kubectl exec -n storage-smoke longhorn-volume-smoke -- cat /data/health
kubectl delete -k kubernetes/smoke/storage

kubectl apply --dry-run=server \
  -f kubernetes/smoke/operators/cnpg-cluster.yaml
kubectl apply --dry-run=server \
  -f kubernetes/smoke/operators/redis-cluster.yaml
```

The storage smoke check creates temporary resources. The two operator checks
are server-side dry runs and do not create database or Redis instances.
