# Ansible and bare-metal hosts

Ansible owns host setup. It does not own objects inside Kubernetes after the API
server starts.

Run every playbook from `ansible/`. That directory's `ansible.cfg` selects
`inventory/hosts.yml`, enables SSH host verification, and loads roles from
`ansible/roles`.

```sh
cd ansible
ansible-galaxy collection install -r requirements.yml
ansible all -m ping
```

The inventory connects as `root` with `~/.ssh/usehivy`. Keep the hosts' SSH
fingerprints in the operator's `known_hosts`; Ansible will stop on an unknown or
changed fingerprint.

## Ownership boundary

| Ansible owns | Direct Kubernetes operations own | Manual or provider-console work owns |
| --- | --- | --- |
| K3s host packages, kernel settings, swap state, nftables, K3s binary and systemd unit | Gateway API CRDs, Cilium, cert-manager, operators | Hetzner bare-metal purchase and OS installation |
| K3s vSwitch Netplan on Kubernetes nodes | namespaces, Services, workloads, policies, storage objects | attaching servers and the Load Balancer to the Hetzner private network |
| runner packages, UFW, BuildKit, CoreDNS, HAProxy, Microsandbox binary and service | application Secrets and ConfigMaps | Hetzner Load Balancer services and target membership |
| restricted SSH accounts used by GitHub Actions | Kustomize application deployments | Vercel application DNS records |
| export of K3s tokens and administrator kubeconfigs to `kubernetes/config/` | rendered chart manifests applied with `kubectl` | object-storage buckets and credentials |

There is no Flux controller, OpenTofu state, or Ansible task that applies the
application manifests. Helm renders Cilium and some operators; `kubectl apply`
owns the resulting objects. Zot is the current exception: its production
instructions use a Helm release.

Do not fix an Ansible-managed host file by hand and stop there. Change the
inventory, variable, task, or template, then run the narrow playbook that owns
it.

## Inventory groups

`k3s_servers` contains every combined K3s server and worker. `k8s0` initializes
embedded etcd; the other members join its private API address.

`k3s_ingress` selects nodes that accept Cilium host-network Gateway traffic.
Membership has two effects:

- the K3s config labels the node `hivy.io/public-ingress=true`;
- runner HAProxy includes the node in its private Gateway and registry pools.

Keep a server in `k3s_ingress` only if it can accept ports 443, 10080, 10443,
and the private NodePorts declared for that host.

`runners` contains the bare-metal Microsandbox machines. Each entry declares a
public SSH address, a private vSwitch address, the control-plane runner name,
and private HTTP URLs for control and preview traffic.

## Local inputs and exports

Ansible reads runner credentials from the ignored file
`kubernetes/config/env/ansible/runners.env`. Create it from the adjacent example
and run the central validator before a deployment:

```sh
cp kubernetes/config/env/ansible/runners.env.example \
  kubernetes/config/env/ansible/runners.env
kubernetes/config/validate.sh
```

The runner deployment requires values for the private control URL, runner join
secret, and runner API token. It can also take a private Zot address and Sentry
DSN. Ansible renders the chosen values to
`/etc/hivy/microsandbox-runner.env` with mode `0600`; the systemd service reads
that file.

The K3s role exports sensitive files after every successful install:

```text
kubernetes/config/kubeconfigs/<inventory-host>/admin.yaml
kubernetes/config/credentials/k3s/<inventory-host>/server-token
kubernetes/config/credentials/k3s/<inventory-host>/node-token
```

Those paths are ignored by Git. Files should remain mode `0600`, and the whole
`kubernetes/config/` directory belongs in the encrypted operator backup.

## K3s host reconciliation

The install playbook targets every host in `k3s_servers` and configures the
restricted GitHub Actions tunnel accounts. An unchanged reconciliation can run
against the full group:

```sh
ansible-playbook playbooks/k3s/install.yml
ansible-playbook playbooks/k3s/validate.yml
```

If a shared K3s setting changed, limit the run to one node, wait for it to
return, then run the next node. The playbook does not declare `serial: 1`, and
its restart handler can otherwise restart both current etcd members together.

Limit a run while adding or repairing one node:

```sh
ansible-playbook playbooks/k3s/install.yml --limit k8s1
ansible-playbook playbooks/k3s/validate.yml --limit k8s1
```

The `k3s-server` role performs these host operations:

- writes `/etc/netplan/90-hetzner-vswitch.yaml` for VLAN 4000 and applies it;
- installs host packages, disables swap, loads `overlay`, `br_netfilter`, and
  `vxlan`, and installs Kubernetes sysctls;
- installs `/etc/nftables.conf`, the pinned K3s binary, command symlinks, visible
  K3s config, and the systemd unit;
- initializes embedded etcd on `k8s0` or joins another server with the exported
  bootstrap token;
- exports the administrator kubeconfig and node credentials locally.

The role's nftables handler runs `nft -f /etc/nftables.conf`; it does not stop
the nftables service, because stopping it would flush Cilium's live rules.

Use the narrow tunnel playbook after changing only the deployment SSH
accounts:

```sh
ansible-playbook playbooks/k3s/deploy-tunnel.yml
```

Each account has no login shell. Its authorized credential permits port
forwarding only to `127.0.0.1:6443` on the target node; Kubernetes RBAC controls
what the tunneled ServiceAccount can change.

## Runner deployment phases

From the repository root, build the Linux runner binary on the operator machine
first:

```sh
make microsandbox-release-linux-amd64
```

Then run the four phases in order:

```sh
cd ansible
ansible-playbook playbooks/phase1-prepare.yml
ansible-playbook playbooks/phase2-install.yml
ansible-playbook playbooks/phase3-deploy.yml
ansible-playbook playbooks/phase4-validate.yml
```

Phase 1 checks for Ubuntu 26.04 amd64 and `/dev/kvm`, installs the Microsandbox
CLI, BuildKit, UFW, runner CoreDNS, and runner HAProxy. It also removes the
retired runner Caddy package, files, and public ports 80 and 443. A runner does
not host Caddy in the current design.

Phase 2 copies `dist/microsandbox-linux-amd64` to
`/usr/local/bin/microsandbox`. Phase 3 measures host capacity, renders the
private environment, installs `microsandbox-runner.service`, and starts it as
root. Phase 4 checks the systemd unit and private health endpoint, calls each
runner from every K3s server, checks subnet-wide UFW rules, and verifies that
public port 8081 is closed. On failure it prints the final 120 runner journal
lines.

Run a narrow reconciliation when only one runner subsystem changed:

```sh
ansible-playbook playbooks/runner-firewall.yml
ansible-playbook playbooks/runner-haproxy.yml
ansible-playbook playbooks/runner-coredns.yml
```

The HAProxy playbook refuses to finish unless every private Gateway backend has
an `UP` entry and a TLS probe reaches Cilium. It also probes Zot TLS. The CoreDNS
playbook verifies every configured private hostname resolves to that runner's
own private address.

## Add a K3s server and worker

Prepare the Ubuntu host and attach it to the same Hetzner vSwitch first. Choose
an unused address in `10.80.1.0/24`; do not reuse a runner or node address.

1. Add the host under `k3s_servers` with `ansible_host`, `k3s_node_name`,
   `k3s_node_ip`, and `k3s_private_interface`.
2. Add it under `k3s_ingress` if it should accept Gateway traffic.
3. Confirm `kubernetes/config/credentials/k3s/k8s0/server-token` exists and has
   mode `0600`.
4. Run the K3s install and validation playbooks with `--limit <host>`.
5. Add the private node address as a target on both Hetzner Load Balancer TCP
   services. Provider-console changes are outside Ansible.
6. Reconcile `playbooks/runner-haproxy.yml`; this updates every runner's pool.
7. Check Cilium, Envoy, node readiness, etcd membership, Longhorn, and workload
   placement before removing an older node.

The K3s template adds every `k3s_servers` private address as a TLS SAN. After the
inventory grows, reconcile each existing server separately and validate it
before starting the next. That updates the common SAN list without asking
Ansible to restart both current etcd members in one handler pass.

Runner UFW already accepts the complete `10.80.1.0/24` segment for runner API
and preview ports. No new runner firewall entry is needed for another K3s node
inside that subnet.

## Add a Microsandbox runner

The runner roles do not create its vSwitch Netplan. Configure and test the new
private interface before Ansible touches the host.

1. Add the runner under `runners` with its SSH address, name, private address,
   and private control and preview URLs.
2. Add that private address to `k3s_private_registry_clients` and
   `k3s_microsandbox_control_clients` in
   `ansible/inventory/group_vars/k3s_servers.yml`.
3. Update the runner CIDR lists in
   `kubernetes/environments/production/zot-exposure.yaml` and
   `kubernetes/environments/production/microsandbox-control.yaml`. Ansible does
   not render either Kubernetes policy.
4. Reconcile K3s nftables one server at a time with the K3s install playbook,
   validate each server, then apply the changed Kubernetes manifests.
5. Run all four runner phases with `--limit <runner>`.
6. Confirm registration in control-plane logs and run a create, preview, wake,
   stop, and delete lifecycle test.

The hard-coded client lists mean adding an inventory entry alone is
insufficient. Zot pulls and control-plane registration will remain blocked
until both host and Cilium policy allow the address.

## Host diagnostics

K3s node:

```sh
ssh -i ~/.ssh/usehivy root@95.216.118.156
systemctl status k3s --no-pager
journalctl -u k3s --since '30 minutes ago' --no-pager
cat /etc/rancher/k3s/config.yaml
nft list table inet hivy_filter
k3s kubectl get nodes -o wide
k3s etcd-snapshot list
```

Runner:

```sh
ssh -i ~/.ssh/usehivy root@135.181.238.109
systemctl status microsandbox-runner hivy-coredns haproxy buildkit --no-pager
journalctl -u microsandbox-runner --since '30 minutes ago' --no-pager
ufw status numbered
ss -lntup
printf 'show stat\n' | socat - UNIX-CONNECT:/run/haproxy/admin.sock
```

Inspect rendered configuration without exposing `/etc/hivy/microsandbox-runner.env`
in tickets or chat. That file contains live authentication material.
