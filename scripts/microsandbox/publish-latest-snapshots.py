#!/usr/bin/env python3
import argparse
import concurrent.futures
import datetime as dt
import json
import os
import random
import shlex
import string
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path


DEFAULT_CONTROL_URL = "https://msb.usehivy.com"
DEFAULT_ORG_ID = "org_system"
DEFAULT_SIZES = "small,medium,large"
DEFAULT_ARCH_SUFFIX = "amd64"
DEFAULT_TIMEOUT_SECONDS = 1800
DEFAULT_CONCURRENCY = 3

GITHUB_LATEST_RELEASE_URL = "https://api.github.com/repos/usehivy/hivy/releases/latest"
RUNTIME_IMAGE = "ghcr.io/usehivy/hivy-sandboxes-runtime"
DEVELOPERS_IMAGE = "ghcr.io/usehivy/hivy-sandboxes-runtime-developers"


def parse_args():
    parser = argparse.ArgumentParser(
        description="Publish durable Microsandbox snapshots for the latest Hivy sandbox images."
    )
    parser.add_argument("--control-url", default=os.environ.get("HIVY_MICROSANDBOX_CONTROL_URL", DEFAULT_CONTROL_URL))
    parser.add_argument("--api-token", default=os.environ.get("HIVY_MICROSANDBOX_API_TOKEN", ""))
    parser.add_argument("--org-id", default=os.environ.get("HIVY_MICROSANDBOX_SNAPSHOT_ORG_ID", DEFAULT_ORG_ID))
    parser.add_argument("--tag", default=os.environ.get("HIVY_MICROSANDBOX_IMAGE_TAG", ""))
    parser.add_argument("--arch-suffix", default=os.environ.get("HIVY_MICROSANDBOX_IMAGE_ARCH_SUFFIX", DEFAULT_ARCH_SUFFIX))
    parser.add_argument(
        "--size",
        default="",
        help="publish one size only; overrides --sizes",
    )
    parser.add_argument("--sizes", default=DEFAULT_SIZES, help="comma-separated sizes to publish")
    parser.add_argument("--concurrency", type=int, default=DEFAULT_CONCURRENCY, help="parallel snapshot publish jobs")
    parser.add_argument("--runtime-only", action="store_true", help="publish only the default runtime image snapshot")
    parser.add_argument("--developers-only", action="store_true", help="publish only the developers image snapshot")
    parser.add_argument("--org-scoped", action="store_true", help="do not mark published snapshots as global")
    parser.add_argument("--skip-verify", action="store_true", help="create snapshots without booting a verification sandbox")
    parser.add_argument(
        "--keep-unverified",
        action="store_true",
        help="keep a snapshot if post-create verification fails; by default unverified snapshots are deleted",
    )
    parser.add_argument(
        "--timeout-seconds",
        type=int,
        default=int(os.environ.get("HIVY_MICROSANDBOX_SNAPSHOT_TIMEOUT_SECONDS", DEFAULT_TIMEOUT_SECONDS)),
    )
    parser.add_argument(
        "--output",
        default=os.environ.get("HIVY_MICROSANDBOX_SNAPSHOT_MANIFEST", ""),
        help="manifest output path; defaults to dist/microsandbox-snapshots-<tag>-<timestamp>.json",
    )
    return parser.parse_args()


def utc_now():
    return dt.datetime.now(dt.UTC).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def short_suffix():
    alphabet = string.ascii_lowercase + string.digits
    return "".join(random.choice(alphabet) for _ in range(8))


def request_json(method, url, token="", body=None, timeout=300, expected=(200,)):
    data = None
    headers = {}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            payload = resp.read()
            if resp.status not in expected:
                raise RuntimeError(f"{method} {url} returned {resp.status}: {payload[:4096]!r}")
            if not payload:
                return {}
            return json.loads(payload.decode("utf-8"))
    except urllib.error.HTTPError as exc:
        payload = exc.read().decode("utf-8", "replace")
        if exc.code in expected:
            return json.loads(payload) if payload else {}
        raise RuntimeError(f"{method} {url} returned {exc.code}: {payload}") from exc


class ControlPlane:
    def __init__(self, base_url, token, timeout_seconds):
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.timeout_seconds = timeout_seconds

    def request(self, method, path, body=None, expected=(200,), timeout=None):
        return request_json(
            method,
            self.base_url + path,
            self.token,
            body,
            timeout=timeout or self.timeout_seconds,
            expected=expected,
        )

    def create_snapshot(self, body):
        return self.request("POST", "/v1/snapshots", body, expected=(201,), timeout=self.timeout_seconds)

    def get_snapshot(self, snapshot_ref):
        try:
            return self.request("GET", f"/v1/snapshots/{snapshot_ref}", expected=(200,), timeout=120)
        except RuntimeError as exc:
            if " returned 404:" in str(exc):
                return {}
            raise

    def delete_snapshot(self, snapshot_id):
        return self.request("DELETE", f"/v1/snapshots/{snapshot_id}", expected=(200, 404), timeout=120)

    def create_sandbox(self, body):
        return self.request("POST", "/v1/sandboxes", body, expected=(201,), timeout=self.timeout_seconds)

    def delete_sandbox(self, sandbox_id):
        return self.request("DELETE", f"/v1/sandboxes/{sandbox_id}", expected=(200, 404), timeout=120)

    def exec(self, sandbox_id, command, timeout_seconds=180):
        return self.request(
            "POST",
            f"/v1/sandboxes/{sandbox_id}/exec",
            {"command": command, "timeout_seconds": timeout_seconds},
            expected=(200,),
            timeout=timeout_seconds + 30,
        )


def resolve_latest_tag(explicit_tag, arch_suffix):
    tag = explicit_tag.strip()
    if not tag:
        headers = {}
        github_token = os.environ.get("GITHUB_TOKEN", "").strip() or os.environ.get("GH_TOKEN", "").strip()
        if github_token:
            headers["Authorization"] = f"Bearer {github_token}"
        req = urllib.request.Request(GITHUB_LATEST_RELEASE_URL, headers=headers)
        with urllib.request.urlopen(req, timeout=60) as resp:
            release = json.loads(resp.read().decode("utf-8"))
        tag = release["tag_name"]
    arch_suffix = arch_suffix.strip()
    if arch_suffix and not tag.endswith("-" + arch_suffix):
        tag = f"{tag}-{arch_suffix}"
    return tag


def image_matrix(tag, runtime_only, developers_only):
    images = []
    if not developers_only:
        images.append(("runtime", f"{RUNTIME_IMAGE}:{tag}"))
    if not runtime_only:
        images.append(("developers", f"{DEVELOPERS_IMAGE}:{tag}"))
    if not images:
        raise SystemExit("--runtime-only and --developers-only cannot both be set")
    return images


def size_matrix(size, sizes):
    if size.strip():
        raw_sizes = [size]
    else:
        raw_sizes = sizes.split(",")
    out = []
    for item in raw_sizes:
        clean = item.strip()
        if clean:
            out.append(clean)
    if not out:
        raise SystemExit("at least one snapshot size is required")
    return out


def pick(payload, *keys, default=None):
    for key in keys:
        if key in payload:
            return payload[key]
    return default


def image_slug(kind, tag, size):
    clean_tag = tag.replace(".", "-").replace(":", "-").replace("/", "-")
    return f"hivy-sandboxes-runtime{'-developers' if kind == 'developers' else ''}-{clean_tag}-{size}"


def snapshot_commands(kind, image, tag, size, marker):
    manifest = json.dumps(
        {
            "kind": kind,
            "image": image,
            "tag": tag,
            "size": size,
            "marker": marker,
            "created_at": utc_now(),
        },
        sort_keys=True,
    )
    commands = [
        "set -eu; mkdir -p /opt/hivy-base-snapshot; "
        f"printf %s {shlex.quote(manifest)} > /opt/hivy-base-snapshot/manifest.json; "
        "cat /opt/hivy-base-snapshot/manifest.json",
        "set -eu; docker info >/opt/hivy-base-snapshot/docker-info.txt; "
        "docker compose version >/opt/hivy-base-snapshot/docker-compose-version.txt",
        "set -eu; node --version >/opt/hivy-base-snapshot/node-version.txt; "
        "npm --version >/opt/hivy-base-snapshot/npm-version.txt; "
        "git --version >/opt/hivy-base-snapshot/git-version.txt; "
        "curl --version >/opt/hivy-base-snapshot/curl-version.txt",
    ]
    if kind == "developers":
        commands.append(
            "set -eu; "
            "bun --version >/opt/hivy-base-snapshot/bun-version.txt; "
            "deno --version >/opt/hivy-base-snapshot/deno-version.txt; "
            "go version >/opt/hivy-base-snapshot/go-version.txt; "
            "ruby --version >/opt/hivy-base-snapshot/ruby-version.txt; "
            "(chromium --version || chromium-browser --version) >/opt/hivy-base-snapshot/chromium-version.txt; "
            "firefox --version >/opt/hivy-base-snapshot/firefox-version.txt"
        )
    return commands


def assert_exec_success(payload, context):
    exit_code = int(pick(payload, "exit_code", "ExitCode", default=0))
    if exit_code == 0:
        return
    stdout = pick(payload, "stdout", "Stdout", default="")
    stderr = pick(payload, "stderr", "Stderr", default="")
    raise RuntimeError(f"{context} failed with exit code {exit_code}\nSTDOUT:\n{stdout}\nSTDERR:\n{stderr}")


def verify_snapshot(control, org_id, kind, image, size, snapshot_id, marker):
    sandbox_id = ""
    started = time.monotonic()
    try:
        sandbox = control.create_sandbox(
            {
                "org_id": org_id,
                "name": f"verify-{kind}-{short_suffix()}",
                "image_ref": image,
                "snapshot_id": snapshot_id,
                "size": size,
                "env": {"HIVY_MICROSANDBOX_SNAPSHOT_MARKER": marker},
                "metadata": {"purpose": "snapshot-publish-verify", "snapshot_id": snapshot_id, "kind": kind},
            }
        )
        create_elapsed = time.monotonic() - started
        sandbox_id = pick(sandbox, "id", "ID")
        if not sandbox_id:
            raise RuntimeError(f"create sandbox response missing id: {sandbox}")
        exec_start = time.monotonic()
        out = control.exec(
            sandbox_id,
            "set -eu; "
            f"grep -F {shlex.quote(marker)} /opt/hivy-base-snapshot/manifest.json; "
            "docker info >/tmp/hivy-snapshot-verify-docker-info.txt; "
            "docker compose version >/tmp/hivy-snapshot-verify-compose-version.txt",
            timeout_seconds=180,
        )
        assert_exec_success(out, "snapshot verification exec")
        return {
            "sandbox_id": sandbox_id,
            "sandbox_create_seconds": round(create_elapsed, 3),
            "exec_verify_seconds": round(time.monotonic() - exec_start, 3),
            "total_verify_seconds": round(time.monotonic() - started, 3),
        }
    finally:
        if sandbox_id:
            try:
                control.delete_sandbox(sandbox_id)
            except Exception as exc:
                print(f"cleanup warning: delete verification sandbox {sandbox_id}: {exc}", file=sys.stderr)


def publish_one(control, args, tag, kind, image, size):
    marker = f"snapshot-publish-{kind}-{int(time.time())}-{short_suffix()}"
    name = image_slug(kind, tag, size)
    existing = control.get_snapshot(name)
    if existing:
        existing_id = pick(existing, "id", "ID")
        existing_image = pick(existing, "base_image_ref", "BaseImageRef")
        existing_status = pick(existing, "status", "Status")
        if existing_image != image:
            raise RuntimeError(f"snapshot alias {name} already points to {existing_image}, not {image}")
        if existing_status != "ready":
            raise RuntimeError(f"snapshot alias {name} exists but is {existing_status}, not ready")
        result = snapshot_result(args, kind, name, existing, image, size, reused=True, snapshot_seconds=0)
        print(f"reusing existing {kind} {size} snapshot {existing_id} for alias {name}", flush=True)
        if not args.skip_verify:
            verification = verify_snapshot(control, args.org_id, kind, image, size, existing_id, marker="")
            result["verified"] = True
            result["verification"] = verification
            print(
                f"verified existing {kind} snapshot {existing_id}: "
                f"sandbox create {verification['sandbox_create_seconds']:.1f}s, "
                f"exec verify {verification['exec_verify_seconds']:.1f}s",
                flush=True,
            )
        return result

    body = {
        "org_id": args.org_id,
        "name": name,
        "alias": name,
        "global": not args.org_scoped,
        "base_image_ref": image,
        "size": size,
        "commands": snapshot_commands(kind, image, tag, size, marker),
        "env": {"HIVY_MICROSANDBOX_SNAPSHOT_MARKER": marker},
    }
    print(f"publishing {kind} snapshot: image={image} size={size} name={name}", flush=True)
    started = time.monotonic()
    snapshot = control.create_snapshot(body)
    snapshot_elapsed = time.monotonic() - started
    snapshot_id = pick(snapshot, "id", "ID")
    if not snapshot_id:
        raise RuntimeError(f"snapshot response missing id: {snapshot}")
    result = snapshot_result(args, kind, name, snapshot, image, size, reused=False, snapshot_seconds=round(snapshot_elapsed, 3))
    print(f"created {kind} snapshot {snapshot_id} in {snapshot_elapsed:.1f}s", flush=True)
    if args.skip_verify:
        return result
    try:
        verification = verify_snapshot(control, args.org_id, kind, image, size, snapshot_id, marker)
        result["verified"] = True
        result["verification"] = verification
        print(
            f"verified {kind} snapshot {snapshot_id}: "
            f"sandbox create {verification['sandbox_create_seconds']:.1f}s, "
            f"exec verify {verification['exec_verify_seconds']:.1f}s",
            flush=True,
        )
    except Exception:
        if not args.keep_unverified:
            try:
                control.delete_snapshot(snapshot_id)
                print(f"deleted unverified snapshot {snapshot_id}", file=sys.stderr, flush=True)
            except Exception as exc:
                print(f"cleanup warning: delete unverified snapshot {snapshot_id}: {exc}", file=sys.stderr)
        raise
    return result


def snapshot_result(args, kind, name, snapshot, image, size, reused, snapshot_seconds):
    return {
        "kind": kind,
        "name": name,
        "alias": pick(snapshot, "Alias", "alias", default=name),
        "global": bool(pick(snapshot, "Global", "global", default=not args.org_scoped)),
        "snapshot_id": pick(snapshot, "id", "ID"),
        "org_id": pick(snapshot, "OrgID", "org_id", default=args.org_id),
        "image_ref": image,
        "size": size,
        "status": pick(snapshot, "Status", "status"),
        "artifact_size_bytes": pick(snapshot, "ArtifactSizeBytes", "artifact_size_bytes", default=0),
        "artifact_url": pick(snapshot, "ArtifactURL", "artifact_url", default=""),
        "artifact_digest": pick(snapshot, "ArtifactDigest", "artifact_digest", default=""),
        "snapshot_digest": pick(snapshot, "SnapshotDigest", "snapshot_digest", default=""),
        "image_manifest_digest": pick(snapshot, "ImageManifestDigest", "image_manifest_digest", default=""),
        "snapshot_seconds": snapshot_seconds,
        "reused": reused,
        "verified": False,
        "verification": None,
    }


def default_output_path(tag):
    stamp = dt.datetime.now(dt.UTC).strftime("%Y%m%dT%H%M%SZ")
    return Path("dist") / f"microsandbox-snapshots-{tag}-{stamp}.json"


def main():
    args = parse_args()
    if not args.api_token.strip():
        raise SystemExit("HIVY_MICROSANDBOX_API_TOKEN or --api-token is required")
    tag = resolve_latest_tag(args.tag, args.arch_suffix)
    images = image_matrix(tag, args.runtime_only, args.developers_only)
    sizes = size_matrix(args.size, args.sizes)
    control = ControlPlane(args.control_url, args.api_token.strip(), args.timeout_seconds)
    manifest = {
        "created_at": utc_now(),
        "control_url": args.control_url.rstrip("/"),
        "org_id": args.org_id,
        "image_tag": tag,
        "sizes": sizes,
        "snapshots": [],
    }
    total_start = time.monotonic()
    jobs = [(kind, image, size) for size in sizes for kind, image in images]
    results = [None] * len(jobs)
    max_workers = max(1, min(args.concurrency, len(jobs)))
    with concurrent.futures.ThreadPoolExecutor(max_workers=max_workers) as executor:
        futures = {
            executor.submit(publish_one, control, args, tag, kind, image, size): idx
            for idx, (kind, image, size) in enumerate(jobs)
        }
        for future in concurrent.futures.as_completed(futures):
            results[futures[future]] = future.result()
    manifest["snapshots"] = results
    manifest["total_seconds"] = round(time.monotonic() - total_start, 3)
    output = Path(args.output) if args.output else default_output_path(tag)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")
    print(f"wrote snapshot manifest: {output}", flush=True)
    print(json.dumps(manifest, indent=2, sort_keys=True), flush=True)


if __name__ == "__main__":
    main()
