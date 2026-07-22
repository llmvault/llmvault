# Platform Engineering Agent access

The `platform-engineering-agent` ServiceAccount is the external, cluster-wide
operational observer. It can inspect production, staging, shared operators,
nodes, events, workload logs, metrics APIs, RBAC configuration, Gateway API,
Cilium, Longhorn, CloudNativePG, Redis, cert-manager, and the observability
operators.

It cannot read Kubernetes Secrets, access `nodes/proxy`, exec or attach to a
container, open a Kubernetes port-forward, impersonate another identity, or
create, patch, update, or delete an object. These exclusions are intentional:
Secret reads and `GET nodes/proxy` are not safely read-only capabilities.

The committed RBAC source is
`kubernetes/access/platform-engineering-agent.yaml`. The Kubernetes API remains
private. Ansible installs the separate
`hivy-deploy-platform-engineering-agent` Unix account on each K3s server; its
SSH key can only forward to `127.0.0.1:6443` and cannot open a shell.

## Generate or rotate the access bundle

Start the administrator SSH tunnel described in `cluster-bootstrap.md`, apply
the RBAC, and reconcile the restricted tunnel account:

```sh
export KUBECONFIG="$PWD/kubernetes/config/kubeconfigs/k8s0/local.yaml"
kubectl apply -f kubernetes/access/platform-engineering-agent.yaml

cd ansible
ansible-playbook playbooks/k3s/deploy-tunnel.yml
cd ..
```

The pinned SSH host keys live in the ignored file
`kubernetes/config/credentials/platform-engineering-agent/known_hosts`. Verify
them through an existing trusted operator connection before writing that file.
Then generate the kubeconfig and environment bundle:

```sh
kubernetes/config/scripts/generate-platform-engineering-agent-access.sh
kubernetes/config/validate.sh
```

The generator creates these mode-`0600`, Git-ignored files:

```text
kubernetes/config/env/platform-engineering/platform-engineering-agent.env
kubernetes/config/kubeconfigs/platform-engineering-agent.yaml
```

Back them up to the approved 1Password vault. The kubeconfig contains a
persistent, read-only ServiceAccount token. Rotate it by deleting and
re-applying `secret/platform-engineering-agent-token`, then regenerate the
bundle. Rotating only the SSH key does not rotate the Kubernetes token, and
vice versa.

## Environment variables

Load every entry from the generated `.env` file into the sandbox or process:

| Variable | Purpose |
| --- | --- |
| `KUBE_CONFIG_B64` | Base64 kubeconfig containing the observer token and cluster CA |
| `K8S_TUNNEL_SSH_KEY_B64` | Base64 restricted SSH private key |
| `K8S_TUNNEL_KNOWN_HOSTS_B64` | Base64 pinned host keys for every tunnel host |
| `K8S_TUNNEL_HOSTS` | Space-separated public K3s server addresses in failover order |
| `K8S_TUNNEL_USER` | Restricted Unix tunnel account |
| `K8S_TUNNEL_LOCAL_PORT` | Local API port; configured as `16443` |
| `K8S_TUNNEL_STATE_DIR` | Private local directory for the tunnel socket and decoded credentials |
| `KUBECONFIG` | Decoded kubeconfig path inside the state directory |
| `KUBECTL_VERSION` | Verified kubectl version installed when kubectl is absent |

The external sandbox should receive only these credentials and its model credential.
Restrict sandbox egress to the two tunnel hosts, the model endpoint,
`monitor.usehivy.com`, and any explicitly required Hivy HTTPS endpoints.

## Start and stop a session

After injecting the environment variables, setup requires no arguments:

```sh
scripts/platform-engineering/setup-kubernetes.sh
kubectl get nodes -o wide
kubectl get pods -A
kubectl top nodes
```

If kubectl is missing, setup downloads the configured release from
`dl.k8s.io`, verifies its published SHA-256 checksum, and installs it into a
directory already present on `PATH`. It decodes credentials with mode `0600`,
tries both K3s nodes, tests `/readyz`, and leaves an SSH control master running
in the background.

Terminate the session when investigation is complete:

```sh
scripts/platform-engineering/terminate-kubernetes.sh
```

Termination closes the SSH control master and removes the locally decoded
kubeconfig, private key, pinned host keys, and connection metadata. It does not
revoke the cluster token or delete the source environment variables held by
the calling sandbox.

## Verify the security boundary

Run these checks as an administrator after applying the RBAC:

```sh
agent_user=system:serviceaccount:platform-engineering:platform-engineering-agent

kubectl auth can-i get pods --all-namespaces --as="$agent_user"
kubectl auth can-i get nodes --as="$agent_user"
kubectl auth can-i get pods --subresource=log --all-namespaces --as="$agent_user"

kubectl auth can-i get secrets --all-namespaces --as="$agent_user"
kubectl auth can-i get nodes --subresource=proxy --as="$agent_user"
kubectl auth can-i create pods --subresource=exec --all-namespaces --as="$agent_user"
kubectl auth can-i create pods --subresource=portforward --all-namespaces --as="$agent_user"
kubectl auth can-i patch deployments --all-namespaces --as="$agent_user"
```

The first three commands must return `yes`; the remaining commands must return
`no`.
