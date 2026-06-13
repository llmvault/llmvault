# Microsandbox Runner Ansible

This Ansible project deploys bare-metal Microsandbox runners only. The Microsandbox control plane runs on Railway and must already be reachable before runner deployment.

## Operator Setup

From the repository root, build the Linux amd64 runner binary:

```sh
make microsandbox-release-linux-amd64
```

Create the local Ansible env file:

```sh
cp ansible/.env.example ansible/.env
```

Fill `ansible/.env` with the Railway control URL, runner secrets, and required R2/S3 snapshot storage settings.

Update `ansible/inventory/hosts.yml` with each runner host:

```yaml
runners:
  hosts:
    runner1:
      ansible_host: 203.0.113.10
      runner_name: runner-1
      runner_public_url: http://203.0.113.10:8081
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
ansible-playbook playbooks/phase3-deploy.yml
ansible-playbook playbooks/phase4-validate.yml
ansible-playbook playbooks/phase5-caddy-proxy.yml
```

Phase 1 prepares Ubuntu 26.04 amd64 hosts, installs Microsandbox with the official installer, configures UFW, and creates `/etc/hivy`.

Phase 2 copies `dist/microsandbox-linux-amd64` to `/usr/local/bin/microsandbox`.

Phase 3 renders `/etc/hivy/microsandbox-runner.env`, installs `microsandbox-runner.service`, and starts the runner.

Phase 4 validates systemd, local health, and public runner health.

Phase 5 provisions the Hetzner Cloud Caddy preview proxy host. It installs Caddy, enables UFW with SSH/HTTP/HTTPS, starts the service, and serves a bootstrap `/health` endpoint. Wildcard preview routing is enabled later after the control-plane route endpoint and DNS challenge credentials are available.

## Railway Requirements

Configure the Railway-hosted Microsandbox control plane separately. The runners need:

- `HIVY_MICROSANDBOX_CONTROL_URL`
- `HIVY_MICROSANDBOX_RUNNER_JOIN_SECRET`
- `HIVY_MICROSANDBOX_RUNNER_API_TOKEN`

The control plane must accept runner registration at `/v1/runners/register`.

## Public Runner API

Runner APIs are intentionally public for now and protected by `HIVY_MICROSANDBOX_RUNNER_API_TOKEN`. UFW opens SSH and the configured runner API port, then denies other inbound traffic.

## Preview Proxy

The preview proxy runs on a Hetzner Cloud VPS and is intentionally separate from Railway. Private networking between the VPS and bare-metal runners is a manual prerequisite and is not managed by this Ansible project.

Bootstrap health check:

```sh
curl http://46.62.169.26/health
```

Phase 5 also installs Redis and a tiny Python route-cache service on the Caddy VPS. Redis is local to the VPS; Caddy calls the local service for route lookups.

Control-plane route push API, exposed through Caddy:

```sh
curl -X PUT http://46.62.169.26/_microsandbox/preview-cache/v1/routes/sbx_123 \
  -H "Authorization: Bearer $HIVY_MICROSANDBOX_PREVIEW_CACHE_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"runner_private_url":"http://10.80.1.2:8081","ports":[3000,5173],"status":"running"}'
```

Bulk push:

```sh
curl -X POST http://46.62.169.26/_microsandbox/preview-cache/v1/routes/bulk \
  -H "Authorization: Bearer $HIVY_MICROSANDBOX_PREVIEW_CACHE_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"routes":[{"sandbox_id":"sbx_123","runner_private_url":"http://10.80.1.2:8081","ports":[3000],"status":"running"}]}'
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
X-Microsandbox-Upstream: http://10.80.1.2:8081
X-Microsandbox-Rewrite-URI: /proxy/sbx_123/3000/path?x=1
```

Until HTTPS is enabled for the preview proxy, do not send production push tokens over the public HTTP endpoint.
