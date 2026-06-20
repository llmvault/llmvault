# Microsandbox Runner Ansible

This Ansible project deploys bare-metal Microsandbox runners only. The Microsandbox control plane runs on Railway and must already be reachable before runner deployment.

## Operator Setup

From the repository root, build the Linux amd64 runner binary. This uses a Docker Linux builder because the Microsandbox Go SDK uses cgo/FFI and cannot be cross-compiled reliably from macOS:

```sh
make microsandbox-release-linux-amd64
```

Create the local Ansible env file:

```sh
cp ansible/.env.example ansible/.env
```

Fill `ansible/.env` with the Railway control URL, runner secrets, and the private Zot registry host used for template images.

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

Update the `preview_proxy` host when provisioning the Caddy preview proxy:

```yaml
preview_proxy:
  hosts:
    caddy:
      ansible_host: 46.62.169.26
      preview_proxy_private_ip: 10.80.0.2
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
ansible-playbook playbooks/phase5-caddy-proxy.yml
```

Phase 1 prepares Ubuntu 26.04 amd64 hosts, installs Microsandbox with the official installer, configures UFW, and creates `/etc/hivy`.

Phase 2 copies `dist/microsandbox-linux-amd64` to `/usr/local/bin/microsandbox`.

Phase 2b provisions Caddy on each runner for the HTTPS runner control API. It verifies that `runner_api_domain` has an A record pointing at the runner public IP before requesting a certificate with the Vercel DNS provider.

Phase 2c provisions Caddy on the private Zot registry host. Caddy serves `registry.usehivy.com:5000` with a public ACME certificate on the private registry IP and proxies to Zot on localhost. Phase 1 maps that registry hostname to the private registry IP on runner hosts.

Phase 3 renders `/etc/hivy/microsandbox-runner.env`, installs `microsandbox-runner.service`, and starts the runner. The runner binds to `127.0.0.1:8081`; public control-plane traffic reaches it through runner-local Caddy over HTTPS.

Phase 4 validates systemd, local health, and public runner health.

Phase 5 provisions the Hetzner Cloud Caddy preview proxy host. It installs Redis, installs the local Microsandbox lifecycle gateway, installs Caddy with the Vercel DNS provider, enables UFW with SSH/HTTP/HTTPS, and serves wildcard preview/runtime routes for `*.preview.usehivy.com`.

## Railway Requirements

Configure the Railway-hosted Microsandbox control plane separately. The runners need:

- `HIVY_MICROSANDBOX_CONTROL_URL`
- `HIVY_MICROSANDBOX_CONTROL_API_TOKEN`
- `HIVY_MICROSANDBOX_RUNNER_JOIN_SECRET`
- `HIVY_MICROSANDBOX_RUNNER_API_TOKEN`

The control plane must accept runner registration at `/v1/runners/register`.

## Runner Network Ports

Runner APIs are exposed through HTTPS Caddy domains such as `runner-0.sandboxes.usehivy.com`. The runner process itself binds to `127.0.0.1:8081`; UFW does not expose `8081` publicly. UFW opens SSH, HTTP, and HTTPS, then denies other inbound traffic.

Preview traffic is separate. Each runner publishes sandbox guest ports onto host ports in `30000-60999`, and UFW allows that range only from the Caddy private IP. With the default 5 preview ports per sandbox, that range supports 6,200 sandboxes per runner before host-port exhaustion.

## Preview Proxy

The preview proxy runs on a Hetzner Cloud VPS and is intentionally separate from Railway. Private networking between the VPS and bare-metal runners is a manual prerequisite and is not managed by this Ansible project.

Bootstrap health check:

```sh
curl http://46.62.169.26/health
```

Phase 5 also installs Redis and the Python Microsandbox gateway service on the Caddy VPS. Redis is local to the VPS; Caddy calls the local service for route lookups, runtime-port wake, and activity reporting. The gateway uses Microsandbox control as the authoritative route/lifecycle source when local Redis is empty, stale, or stopped.

Route admin API, exposed through Caddy for compatibility:

```sh
curl -X PUT http://46.62.169.26/_microsandbox/preview-cache/v1/routes/sbx_123 \
  -H "Authorization: Bearer $HIVY_MICROSANDBOX_PREVIEW_CACHE_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"sandbox_id":"sbx_123","status":"running","upstreams":{"3000":"http://10.80.1.2:43122","5173":"http://10.80.1.2:45173"}}'
```

Bulk push:

```sh
curl -X POST http://46.62.169.26/_microsandbox/preview-cache/v1/routes/bulk \
  -H "Authorization: Bearer $HIVY_MICROSANDBOX_PREVIEW_CACHE_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"routes":[{"sandbox_id":"sbx_123","status":"running","upstreams":{"3000":"http://10.80.1.2:43122"}}]}'
```

Delete:

```sh
curl -X DELETE http://46.62.169.26/_microsandbox/preview-cache/v1/routes/sbx_123 \
  -H "Authorization: Bearer $HIVY_MICROSANDBOX_PREVIEW_CACHE_TOKEN"
```

Caddy lookup is local-only:

```text
GET http://127.0.0.1:8091/v1/lookup
X-Forwarded-Host: 3000-sbx_123.preview.usehivy.com
X-Forwarded-Uri: /path?x=1
```

It returns:

```text
X-Microsandbox-Upstream: 10.80.1.2:43122
```

Route payloads store upstreams as full URLs. The lookup service returns only `host:port` because Caddy's dynamic `reverse_proxy` upstream expects a dial address. Caddy preserves the original request path and query without a rewrite.

## Flagship E2E

After the control plane, preview proxy, and at least one runner are deployed, run the canonical Vite preview lifecycle test from the repository root:

```sh
export HIVY_MICROSANDBOX_CONTROL_URL=https://msb.usehivy.com
export HIVY_MICROSANDBOX_API_TOKEN=...
export HIVY_MICROSANDBOX_E2E_PREVIEW_RESOLVE_IP=46.62.169.26 # optional when DNS is already fresh

scripts/microsandbox/e2e-vite-preview.py --size medium
```

The script creates a sandbox, installs and starts Vite on port `5173`, waits for preview `200`, stops the sandbox, verifies the next preview request auto-wakes back to `200`, then deletes it and waits for `404`.
