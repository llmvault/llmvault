# Infrastructure Ansible

Ansible has two deliberately narrow responsibilities in this repository:

- bootstrap the bare-metal Microsandbox hosts;
- install and host-configure K3s on Kubernetes server nodes.

Ansible does not install cluster add-ons or apply application workloads. Once
the Kubernetes API is available, those resources are managed as ordinary
Kustomize manifests and applied directly with `kubectl`.

## K3s server

The production K3s inventory group and pinned settings live in
`inventory/hosts.yml` and `inventory/group_vars/k3s_servers.yml`. From this
directory, install or reconcile the host and then validate it:

```sh
ansible-playbook playbooks/k3s/install.yml --limit k8s0
ansible-playbook playbooks/k3s/validate.yml --limit k8s0
```

The install playbook exports the administrator kubeconfig, K3s server token,
and node token to `.secrets/k8s0/`. This entire directory is git-ignored. Once
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
      runner_api_domain: runner-1.sandboxes.usehivy.com
      runner_public_url: https://runner-1.sandboxes.usehivy.com
      runner_preview_base_url: http://10.80.1.2
```

## Phases

Run phases from the `ansible/` directory:

```sh
ansible-playbook playbooks/phase1-prepare.yml
ansible-playbook playbooks/phase2-install.yml
ansible-playbook playbooks/phase2b-runner-caddy.yml
ansible-playbook playbooks/phase2c-registry-proxy.yml
ansible-playbook playbooks/phase3-deploy.yml
ansible-playbook playbooks/phase4-validate.yml
```

Phase 1 prepares Ubuntu 26.04 amd64 hosts, installs Microsandbox with the official installer, configures UFW, and creates `/etc/hivy`.

Phase 2 copies `dist/microsandbox-linux-amd64` to `/usr/local/bin/microsandbox`.

Phase 2b provisions Caddy on each runner for the HTTPS runner control API. It verifies that `runner_api_domain` has an A record pointing at the runner public IP before requesting a certificate with the Vercel DNS provider.

Phase 2c provisions Caddy on the private Zot registry host. Caddy serves `registry.usehivy.com:5000` with a public ACME certificate on the private registry IP and proxies to Zot on localhost. Phase 1 maps that registry hostname to the private registry IP on runner hosts.

Phase 3 renders `/etc/hivy/microsandbox-runner.env`, installs `microsandbox-runner.service`, and starts the runner. The runner binds to `127.0.0.1:8081`; public control-plane traffic reaches it through runner-local Caddy over HTTPS.

Phase 4 validates systemd, local health, and public runner health.

Wildcard preview traffic is served by the Kubernetes Gateway API, Cilium Envoy,
the in-cluster Caddy proxy, and the in-cluster Microsandbox preview cache.

## Control-plane requirements

Configure the Kubernetes-hosted Microsandbox control plane separately. The runners need:

- `HIVY_MICROSANDBOX_CONTROL_URL`
- `HIVY_MICROSANDBOX_CONTROL_API_TOKEN`
- `HIVY_MICROSANDBOX_RUNNER_JOIN_SECRET`
- `HIVY_MICROSANDBOX_RUNNER_API_TOKEN`

The control plane must accept runner registration at `/v1/runners/register`.

## Runner Network Ports

Runner APIs bind to their private vSwitch addresses on port `8081`; UFW allows
that port only from the Kubernetes node. UFW opens SSH, HTTP, and HTTPS, then
denies other inbound traffic.

Preview traffic is separate. Each runner publishes sandbox guest ports onto
host ports in `30000-60999`, and UFW allows that range only from the Kubernetes
node. With the default 5 preview ports per sandbox, that range supports 6,200
sandboxes per runner before host-port exhaustion.

## Preview proxy

The preview cache, Redis, and Caddy proxy run in the production Kubernetes
namespace. Cilium Gateway API exposes them through the Hetzner load balancer;
the proxy reaches runner preview ports over the private vSwitch only. Their
manifests and operating notes live in `../kubernetes/environments/production/`.

## Flagship E2E

After the control plane, preview proxy, and at least one runner are deployed, run the canonical Vite preview lifecycle test from the repository root:

```sh
export HIVY_MICROSANDBOX_CONTROL_URL=http://10.80.1.4:32080
export HIVY_MICROSANDBOX_API_TOKEN=...

scripts/microsandbox/e2e-vite-preview.py --size medium
```

The script creates a sandbox, installs and starts Vite on port `5173`, waits for preview `200`, stops the sandbox, verifies the next preview request auto-wakes back to `200`, then deletes it and waits for `404`.
