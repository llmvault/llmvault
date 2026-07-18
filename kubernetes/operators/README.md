# Production Kubernetes operators

These control-plane components are installed as pinned, rendered manifests and
owned with direct `kubectl apply`. Helm is used only as a renderer; there are no
Helm releases and no GitOps controller.

## Versions

- Longhorn: `v1.12.0`
- CloudNativePG: `v1.30.0`
- Barman Cloud plugin: `v0.13.0`
- OT Redis Operator: `v0.26.0`

Generated upstream manifests and source archives live under the git-ignored
`ansible/.secrets/k8s0/operators/` directory.

Apply the operator namespaces first:

```sh
kubectl apply -f kubernetes/operators/namespaces.yaml
```

## Longhorn host prerequisites

Longhorn uses the V1 data engine. Run its official preflight installer and
checker before applying the chart:

```sh
longhornctl --kubeconfig "$KUBECONFIG" \
  --image longhornio/longhorn-cli:v1.12.0 install preflight

longhornctl --kubeconfig "$KUBECONFIG" check preflight
```

The current bare-metal node uses local NVMe RAID1 and has no multipath maps.
`multipathd.service` and `multipathd.socket` are masked because Longhorn devices
must not be claimed by the unused multipath daemon. `iscsid` is enabled and
running. The preflight installer provides `nfs-common`, `nfs`, and `dm_crypt`.

Persist the required modules and services without adding them to Ansible:

```sh
scp -i ~/.ssh/usehivy \
  kubernetes/operators/longhorn/host-modules.conf \
  root@95.216.118.156:/etc/modules-load.d/longhorn.conf

ssh -i ~/.ssh/usehivy root@95.216.118.156 \
  'modprobe nfs && modprobe dm_crypt && modprobe iscsi_tcp && \
   systemctl enable --now iscsid && \
   systemctl disable --now multipathd.service multipathd.socket && \
   systemctl mask multipathd.service multipathd.socket'
```

The only expected preflight warning on the one-node cluster is that CoreDNS has
one replica. Increase CoreDNS to at least two replicas after adding another
node.

## Render and apply Longhorn

The chart checksum is pinned to prevent an upstream package replacement:

```sh
mkdir -p ansible/.secrets/k8s0/operators

curl -fsSL \
  https://github.com/longhorn/charts/releases/download/longhorn-1.12.0/longhorn-1.12.0.tgz \
  -o ansible/.secrets/k8s0/operators/longhorn-1.12.0.tgz

echo "869bb20701b154473606f1e8967b27f34f2448a2dfe6eb8970f1cae6957384f5  ansible/.secrets/k8s0/operators/longhorn-1.12.0.tgz" \
  | shasum -a 256 -c -

helm template longhorn \
  ansible/.secrets/k8s0/operators/longhorn-1.12.0.tgz \
  --namespace longhorn-system \
  --include-crds \
  --no-hooks \
  --values kubernetes/operators/longhorn/values.yaml \
  > ansible/.secrets/k8s0/operators/longhorn-v1.12.0.yaml

kubectl apply --server-side=true --field-manager=hivy-operators \
  -f ansible/.secrets/k8s0/operators/longhorn-v1.12.0.yaml
```

The default StorageClass has one replica while the cluster has one node. When
nodes are added, raise both replica-count settings and the CSI controller
replicas before provisioning production volumes that need node redundancy.

## Apply CloudNativePG

```sh
curl -fsSL \
  https://github.com/cloudnative-pg/cloudnative-pg/releases/download/v1.30.0/cnpg-1.30.0.yaml \
  -o ansible/.secrets/k8s0/operators/cnpg-v1.30.0.yaml

echo "f8bede43fe4ee0d478c2355b204a36876b2ae4faac60f2a9452280b293da3b88  ansible/.secrets/k8s0/operators/cnpg-v1.30.0.yaml" \
  | shasum -a 256 -c -

kubectl apply --server-side=true --field-manager=hivy-operators \
  -f ansible/.secrets/k8s0/operators/cnpg-v1.30.0.yaml
```

## Apply the CloudNativePG Barman Cloud plugin

CloudNativePG delegates native PostgreSQL base backups, continuous WAL
archiving, retention, and recovery to the Barman Cloud plugin. Install the
plugin after the CloudNativePG controller and before applying any Cluster that
declares it as a WAL archiver:

```sh
curl -fsSL \
  https://github.com/cloudnative-pg/plugin-barman-cloud/releases/download/v0.13.0/manifest.yaml \
  -o ansible/.secrets/k8s0/operators/barman-cloud-v0.13.0.yaml

echo "d2e71e7b06822448f1a421f05781846cfdb9cc621e7ef32eef5e20c5133213b0  ansible/.secrets/k8s0/operators/barman-cloud-v0.13.0.yaml" \
  | shasum -a 256 -c -

kubectl apply --server-side=true --field-manager=hivy-operators \
  -f ansible/.secrets/k8s0/operators/barman-cloud-v0.13.0.yaml

kubectl -n cnpg-system rollout status deployment/barman-cloud --timeout=5m
```

## Render and apply OT Redis Operator

The OT operator is used because it supports Redis Cluster sharding as well as
standalone, replication, and Sentinel topologies. The webhook is deliberately
disabled; the CRD schemas and operator reconciliation are sufficient for the
planned Redis Cluster deployment.

```sh
curl -fsSL \
  https://github.com/OT-CONTAINER-KIT/redis-operator/archive/refs/tags/v0.26.0.tar.gz \
  -o ansible/.secrets/k8s0/operators/redis-operator-v0.26.0.tar.gz

echo "0389653cc2235ba1149870be75c5e20391760ee0c9eb71283478ab23171ebfff  ansible/.secrets/k8s0/operators/redis-operator-v0.26.0.tar.gz" \
  | shasum -a 256 -c -

tar -xzf ansible/.secrets/k8s0/operators/redis-operator-v0.26.0.tar.gz \
  -C ansible/.secrets/k8s0/operators

helm template redis-operator \
  ansible/.secrets/k8s0/operators/redis-operator-0.26.0/charts/redis-operator \
  --namespace redis-operator \
  --include-crds \
  --no-hooks \
  --values kubernetes/operators/redis-operator/values.yaml \
  > ansible/.secrets/k8s0/operators/redis-operator-v0.26.0.yaml

kubectl apply --server-side=true --field-manager=hivy-operators \
  -f ansible/.secrets/k8s0/operators/redis-operator-v0.26.0.yaml
```

## Validation

```sh
kubectl -n longhorn-system get pods
kubectl -n cnpg-system rollout status deployment/cnpg-controller-manager
kubectl -n redis-operator rollout status deployment/redis-operator

kubectl apply -k kubernetes/smoke/storage
kubectl wait -n storage-smoke --for=condition=Ready pod/longhorn-volume-smoke \
  --timeout=5m
kubectl exec -n storage-smoke longhorn-volume-smoke -- cat /data/health
kubectl delete -k kubernetes/smoke/storage

kubectl apply --dry-run=server -f kubernetes/smoke/operators/cnpg-cluster.yaml
kubectl apply --dry-run=server -f kubernetes/smoke/operators/redis-cluster.yaml
```

The storage smoke resources are temporary. No PostgreSQL or Redis instances are
created by these checks.
