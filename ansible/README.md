# Infrastructure Ansible

Ansible has two deliberately narrow responsibilities in this repository:

- bootstrap the bare-metal Microsandbox hosts;
- install and host-configure K3s on Kubernetes server nodes.

Ansible does not install cluster add-ons or apply application workloads. Once
the Kubernetes API is available, those resources are managed as ordinary
Kustomize manifests and applied directly with `kubectl`.

Install the operator-side collection dependencies once before running the
playbooks:

```sh
ansible-galaxy collection install -r requirements.yml
```

## K3s server

The production K3s inventory group and pinned settings live in
`inventory/hosts.yml` and `inventory/group_vars/k3s_servers.yml`. From this
directory, install or reconcile the host and then validate it:

```sh
ansible-playbook playbooks/k3s/install.yml --limit k8s0
ansible-playbook playbooks/k3s/validate.yml --limit k8s0
```

`k8s0` is the bootstrap embedded-etcd server. To add another combined
control-plane/worker node, add it to both `k3s_servers` and `k3s_ingress`, give
it a unique `k3s_node_ip`, then limit the same playbooks to the new inventory
host. The role configures its Hetzner vSwitch VLAN, copies the exported shared
server token, and joins it to the bootstrap API instead of initializing another
cluster. Re-apply `playbooks/runner-haproxy.yml` and add the node's private IP to
the Hetzner load balancer after validation. Runner UFW rules cover the complete
`10.80.1.0/24` vSwitch subnet, so adding a node in that subnet does not require
another firewall rule.

The install playbook exports each server's administrator kubeconfig, K3s server
token, and node token to `.secrets/<inventory-host>/`. This entire directory is
git-ignored. Once
cluster add-ons or workloads have created Kubernetes Secrets, export those
separately from the repository root:

```sh
kubectl get secrets --all-namespaces -o yaml \
  > ansible/.secrets/k8s0/cluster-secrets.yaml
chmod 600 ansible/.secrets/k8s0/*
```

Keep these files in an encrypted operator backup. They are sensitive recovery
material, not a declarative secret-management system. The post-K3s bootstrap
and access procedure is documented in `../kubernetes/bootstrap/README.md`.

## Microsandbox runners

These playbooks deploy the bare-metal Microsandbox runners. The Microsandbox
control plane runs in the production Kubernetes namespace and must already be
reachable on its private NodePort before runner deployment.

## Operator Setup

From the repository root, build the Linux amd64 runner binary. This uses a Docker Linux builder because the Microsandbox Go SDK uses cgo/FFI and cannot be cross-compiled reliably from macOS:

```sh
make microsandbox-release-linux-amd64
```

Create the local Ansible env file:

```sh
cp ansible/.env.example ansible/.env
```

Fill `ansible/.env` with the private Kubernetes control URL, runner secrets, and
the private Zot registry host used for template images.

Update `ansible/inventory/hosts.yml` with each runner host:

```yaml
runners:
  hosts:
    runner1:
      ansible_host: 203.0.113.10
      runner_name: runner-1
      runner_private_ip: 10.80.1.2
      runner_public_url: http://10.80.1.2:8081
      runner_preview_base_url: http://10.80.1.2
```

## Phases

Run phases from the `ansible/` directory:

```sh
ansible-playbook playbooks/phase1-prepare.yml
ansible-playbook playbooks/phase2-install.yml
ansible-playbook playbooks/phase3-deploy.yml
ansible-playbook playbooks/phase4-validate.yml
```

Phase 1 prepares Ubuntu 26.04 amd64 hosts, installs Microsandbox with the official installer, removes the retired runner API Caddy proxy, configures UFW, creates `/etc/hivy`, and installs runner-local HAProxy and CoreDNS. CoreDNS resolves the production and staging API hostnames to that runner's private HAProxy listener. HAProxy passes TLS through to healthy nodes listed in the explicit `k3s_ingress` inventory group. Runner hosts and sandbox DNS proxies use the same resolver on standard DNS port 53.

To reconcile only runner firewall rules, including after changing the private
vSwitch subnet, run `ansible-playbook playbooks/runner-firewall.yml`.

Phase 2 copies `dist/microsandbox-linux-amd64` to `/usr/local/bin/microsandbox`.

Phase 3 renders `/etc/hivy/microsandbox-runner.env`, installs `microsandbox-runner.service`, and starts the runner. The runner binds to its private vSwitch address on port `8081`; the Kubernetes control plane reaches it directly over the private network.

Phase 4 validates systemd, private health, and that port `8081` is not publicly reachable.

Every Kubernetes node that should accept private Gateway traffic must be a
member of the `k3s_ingress` inventory group and define `k3s_node_ip`. That one
membership both labels the node for Cilium host-network ingress and adds it to
the HAProxy backend pool on every runner. After changing that group, apply the
runner pool directly with `ansible-playbook playbooks/runner-haproxy.yml`.

Zot runs inside the production Kubernetes namespace. Each runner maps
`registry.usehivy.com` to its own private HAProxy listener on port `5000`, and
HAProxy passes TLS through to Zot's private Kubernetes NodePort on `32500`. The
K3s host firewall accepts that NodePort only from runner private IPs. Deployment
values and operating instructions live in
`../kubernetes/environments/production/`.

Wildcard preview traffic is served by the Kubernetes Gateway API, Cilium Envoy,
the in-cluster Caddy proxy, and the in-cluster Microsandbox preview cache.
Kubernetes CoreDNS rewrites `*.preview.usehivy.com` to a private HAProxy bridge.
The bridge prepends PROXY v2 before forwarding to Cilium, so API and asynq Pods
use the same public HTTPS URLs without leaving the cluster.

## Control-plane requirements

Configure the Kubernetes-hosted Microsandbox control plane separately. The runners need:

- `HIVY_MICROSANDBOX_CONTROL_URL`
- `HIVY_MICROSANDBOX_CONTROL_API_TOKEN`
- `HIVY_MICROSANDBOX_RUNNER_JOIN_SECRET`
- `HIVY_MICROSANDBOX_RUNNER_API_TOKEN`

The control plane must accept runner registration at `/v1/runners/register`.

## Runner Network Ports

Runner APIs bind to their private vSwitch addresses on port `8081`; UFW allows
that port only from the Kubernetes bare-metal vSwitch subnet. UFW opens SSH,
HTTP, and HTTPS, then denies other inbound traffic.

Preview traffic is separate. Each runner publishes sandbox guest ports onto
host ports in `30000-60999`, and UFW allows that range only from the Kubernetes
bare-metal vSwitch subnet. This automatically covers future nodes without
opening the ports to the public network. With the default 5 preview ports per
sandbox, that range supports 6,200 sandboxes per runner before host-port
exhaustion.

## Preview proxy

The preview cache, Redis, and Caddy proxy run in the production Kubernetes
namespace. Cilium Gateway API exposes the public path through the Hetzner load
balancer, while in-cluster callers resolve preview names to the production
preview TLS bridge. The bridge forwards to Cilium with PROXY v2, and the proxy
reaches runner preview ports over the private
vSwitch only. Their
manifests and operating notes live in `../kubernetes/environments/production/`.

## Flagship E2E

After the control plane, preview proxy, and at least one runner are deployed, run the canonical Vite preview lifecycle test from the repository root:

```sh
export HIVY_MICROSANDBOX_CONTROL_URL=http://10.80.1.4:32080
export HIVY_MICROSANDBOX_API_TOKEN=...

scripts/microsandbox/e2e-vite-preview.py --size medium
```

The script creates a sandbox, installs and starts Vite on port `5173`, waits for preview `200`, stops the sandbox, verifies the next preview request auto-wakes back to `200`, then deletes it and waits for `404`.
