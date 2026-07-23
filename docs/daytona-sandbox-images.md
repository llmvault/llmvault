# Daytona sandbox images

Hivy previously published two Daytona images:

```text
ghcr.io/usehivy/hivy-sandboxes-runtime-daytona:<release>
ghcr.io/usehivy/hivy-sandboxes-runtime-developers-daytona:<release>
```

The app image was not part of this path. Automated image and snapshot
publication has been retired: no GitHub Actions workflow builds these targets,
pushes them to a registry, creates release manifests, or uploads them to
Daytona.

## Image contract

Both Dockerfiles have a `daytona` target and a `microsandbox` target. The final
target remains `microsandbox`, so callers that do not select a target keep the
old image behavior.

The Daytona targets use this contract:

- The sandbox user is `daytona` with UID and GID 1000.
- `HOME` and `HIVY_WORKSPACE_ROOT` are `/home/daytona`.
- Runtime state is under `/home/daytona/.hivy`.
- Repositories are under `/home/daytona/repos`.
- `/workspace` is a symlink to `/home/daytona` for config and tool compatibility.
- The snapshot entrypoint is `/usr/local/bin/hivy-daytona-entrypoint`.
- The entrypoint starts `hivy-sandboxes-runtime` directly. It does not start or
  depend on systemd.
- The developer image starts a root Docker daemon through one fixed,
  passwordless `sudo` rule before the Hivy runtime. Daytona's container DinD
  support requires that daemon model. The sandbox user, Runtime, and agent
  commands remain UID 1000; `DOCKER_HOST` points to a `docker`-group socket.
  The daemon uses `overlay2` with its root-owned graph data at Daytona's
  dedicated `/var/lib/docker` XFS mount. The sandbox root filesystem is itself
  overlay-backed, so neither a nested `overlay2` data root under the user's home
  nor `fuse-overlayfs` can start nested containers there.

The Hivy systemd service and environment generator are installed only in the
`microsandbox` target. Some desktop or browser packages may bring systemd
libraries into the package graph; Daytona does not run systemd as PID 1 and no
Hivy process depends on it.

This matches Daytona's documented `/home/daytona` user layout and its support
for custom snapshot entrypoints. Daytona also documents Docker-in-Docker and
Docker Compose in snapshots:

- <https://www.daytona.io/docs/en/snapshots/>
- <https://www.daytona.io/docs/en/declarative-builder/>
- <https://www.daytona.io/docs/en/process-code-execution/>

## Provider behavior

The Daytona driver translates a versioned Hivy runtime image and resource tuple
to an immutable Daytona snapshot name. Other providers still receive the normal
image reference.

```text
hivy-sandboxes-runtime-daytona-<version-with-dashes>-<size>-v1
hivy-sandboxes-runtime-developers-daytona-<version-with-dashes>-<size>-v1
```

For example:

```text
hivy-sandboxes-runtime-daytona-7-2-1-small-v1
hivy-sandboxes-runtime-developers-daytona-7-2-1-small-v1
```

Release snapshots use these allocations:

| Size | CPU | Memory (GiB) | Default disk (GiB) | Developer disk (GiB) |
| --- | ---: | ---: | ---: | ---: |
| micro | 1 | 1 | 5 | 10 |
| nano | 1 | 1 | 5 | 10 |
| small | 1 | 2 | 10 | 10 |
| medium | 2 | 4 | 10 | 10 |
| large | 4 | 8 | 10 | 10 |
| xlarge | 4 | 8 | 10 | 10 |

Hivy's `micro` size asks other providers for a sub-GiB memory allocation. The
Daytona API represents memory in whole GiB, so the Daytona provider selects the
smallest supported allocation, 1 CPU, 1 GiB memory, and 5 GiB disk, and writes an
info log when it makes that adjustment. The developer image needs at least a 10
GiB disk because Daytona applies the disk value to the full sandbox filesystem.
Daytona's standard per-sandbox limit is 4 vCPU, 8 GiB memory, and 10 GiB disk.
Medium and large retain their Hivy CPU and memory allocations while their disks
are capped at 10 GiB. Xlarge maps to Daytona's maximum 4/8/10 allocation. Set
`HIVY_DAYTONA_MAX_CPU`, `HIVY_DAYTONA_MAX_MEMORY_GB`, and
`HIVY_DAYTONA_MAX_DISK_GB` if the Daytona account has custom per-sandbox limits.
The driver fails on unknown resource tuples; it does not silently select a
different snapshot.

For every sandbox creation, the driver sets the `daytona` user and Hivy paths,
keeps the preview private, and sends `networkBlockAll=false`. Organization-tier
firewall rules still take precedence. Daytona's network rules are documented at
<https://www.daytona.io/docs/en/network-limits/>.

Custom Hivy templates also stay on the snapshot path. The provider creates a
temporary private sandbox from the matching version-and-size runtime snapshot,
runs the template build commands as `daytona`, clears transient runtime state,
stops it, and captures its filesystem as the template snapshot. It then deletes
the temporary sandbox. This avoids asking Daytona's remote builder to pull the
Hivy base image from GHCR. Explicit non-Hivy base images still use Daytona's
normal declarative image builder. Daytona documents cold filesystem capture at
<https://www.daytona.io/docs/en/sandboxes/>.

Runtime URLs use a signed private preview URL with the maximum 86,400-second
expiry. The token is embedded in the URL and remains valid across restarts until
it expires. Hivy refreshes and persists another URL before its own shorter cache
expires. See <https://www.daytona.io/docs/en/preview/>.

## Publication status

The former release, runtime-manifest, and promotion workflows have been
removed. Publishing a GitHub Release does not build or push Daytona images,
publish snapshots, or update provider configuration.

### First provider cutover

Provider external IDs are not portable. Before the first production switch,
inspect `sandboxes`, `sandbox_templates`, and `sandbox_warm_slots` for rows owned
by another provider. Do not relabel those rows as Daytona resources.

Delete each old provider resource through its original provider, detach a
session from its old sandbox row, revoke the old sandbox-scoped runtime token,
and delete the old sandbox row in one database transaction. The session events
remain in place; its next delivery creates and attaches a new Daytona sandbox.
Rebuild any custom template through Daytona before assigning it to an agent.
After this one-time drain, update provider configuration explicitly; no release
workflow performs that change.

## Acceptance checks

Test one default snapshot and one developer snapshot before switching Railway:

```bash
set -a
source .env
set +a

go run ./cmd/verify-devbox \
  -snapshot hivy-sandboxes-runtime-daytona-7-2-1-small-v1

go run ./cmd/verify-devbox \
  -snapshot hivy-sandboxes-runtime-developers-daytona-7-2-1-small-v1
```

The verifier deletes its sandbox unless `-keep` is passed. It checks:

- the non-root user, home directory, and `/workspace` compatibility link;
- direct runtime startup without systemd;
- outbound network access;
- a private 24-hour signed preview URL;
- `/healthz`, config upload, then `/readyz`;
- Daytona process command execution;
- Docker absence in the default image; and
- Docker plus a real `docker compose up`, logs, and teardown in the
  developer image.

After the Railway deployment, confirm both services are healthy, verify that the
provider and release tag variables match, and create a short-lived agent session.
Check API and worker logs for Daytona creation, signed-preview, config-push, or
ready-check errors before removing the old provider credentials.
