# Daytona sandbox images

Hivy publishes two Daytona images:

```text
ghcr.io/usehivy/hivy-sandboxes-runtime-daytona:<release>
ghcr.io/usehivy/hivy-sandboxes-runtime-developers-daytona:<release>
```

The app image is not part of this path.

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
  The daemon uses `overlay2`; `fuse-overlayfs` cannot remount nested container
  root filesystems in Daytona's container sandbox.

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

## Release setup

Add one GitHub Actions secret:

```text
HIVY_DAYTONA_API_KEY
```

Add these GitHub Actions variables:

```text
HIVY_DAYTONA_API_URL=https://app.daytona.io/api
HIVY_DAYTONA_TARGET=<organization target or empty>
```

The key needs snapshot and sandbox read/write permissions. Keep the key out of
workflow output and release artifacts.

The release workflow performs this sequence:

1. Build the Linux AMD64 runtime and canvas binaries.
2. Build and push the existing Microsandbox images.
3. Build the `daytona` target from each Dockerfile and push the two Daytona
   images with immutable release tags.
4. Write all four runtime image references to `release-manifest.json`.
5. Pull the exact Daytona AMD64 tags on the Actions runner.
6. Run `daytona snapshot push` for default and developer variants, with the Hivy
   entrypoint and each supported size.
7. Wait until every snapshot is `active`.
8. Launch the default and developer `small` snapshots and run the acceptance
   verifier, including a Docker Compose workload in the developer sandbox.
9. Set the API and worker Railway services to `HIVY_SANDBOX_PROVIDER_ID=daytona`
   and write the Daytona connection variables.

Railway is not changed if image publication or snapshot activation fails.
Prereleases also build and verify their snapshots but do not update Railway.

Daytona previously returned `denied` while pulling Hivy's public GHCR packages
through its remote image path. The release job therefore authenticates to GHCR,
pulls the images onto the Actions runner, and uses `daytona snapshot push` to
upload the local AMD64 image. This is also the path Daytona documents for local
images.

### First provider cutover

Provider external IDs are not portable. Before the first production switch,
inspect `sandboxes`, `sandbox_templates`, and `sandbox_warm_slots` for rows owned
by another provider. Do not relabel those rows as Daytona resources.

Delete each old provider resource through its original provider, detach a
session from its old sandbox row, revoke the old sandbox-scoped runtime token,
and delete the old sandbox row in one database transaction. The session events
remain in place; its next delivery creates and attaches a new Daytona sandbox.
Rebuild any custom template through Daytona before assigning it to an agent.
After this one-time drain, the release workflow can update both Railway services.

## Manual publication

Load the Daytona values without printing the key, install the Daytona CLI, and
log Docker into GHCR. Then download the release manifest and run the same script
used in Actions:

```bash
tag=v7.2.1
tmpdir=$(mktemp -d)
gh release download "$tag" --pattern release-manifest.json --dir "$tmpdir"

set -a
source .env
set +a

bash scripts/release/publish-daytona-snapshots.sh \
  "$tmpdir/release-manifest.json"
```

The script is idempotent for active snapshots. It refuses to overwrite a
versioned snapshot in any other state. Delete a failed test snapshot explicitly
or publish a new recipe suffix instead of changing an active release snapshot.

To republish selected variants or sizes through Actions, run
`promote-sandbox-release.yml`. Its inputs support `micro`, `nano`, `small`,
`medium`, `large`, `xlarge`, or `all`.

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
