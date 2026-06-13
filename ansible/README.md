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

## Phases

Run phases from the `ansible/` directory:

```sh
ansible-playbook playbooks/phase1-prepare.yml
ansible-playbook playbooks/phase2-install.yml
ansible-playbook playbooks/phase3-deploy.yml
ansible-playbook playbooks/phase4-validate.yml
```

Phase 1 prepares Ubuntu 26.04 amd64 hosts, installs Microsandbox with the official installer, configures UFW, and creates `/etc/hivy`.

Phase 2 copies `dist/microsandbox-linux-amd64` to `/usr/local/bin/microsandbox`.

Phase 3 renders `/etc/hivy/microsandbox-runner.env`, installs `microsandbox-runner.service`, and starts the runner.

Phase 4 validates systemd, local health, and public runner health.

## Railway Requirements

Configure the Railway-hosted Microsandbox control plane separately. The runners need:

- `HIVY_MICROSANDBOX_CONTROL_URL`
- `HIVY_MICROSANDBOX_RUNNER_JOIN_SECRET`
- `HIVY_MICROSANDBOX_RUNNER_API_TOKEN`

The control plane must accept runner registration at `/v1/runners/register`.

## Public Runner API

Runner APIs are intentionally public for now and protected by `HIVY_MICROSANDBOX_RUNNER_API_TOKEN`. UFW opens SSH and the configured runner API port, then denies other inbound traffic.
