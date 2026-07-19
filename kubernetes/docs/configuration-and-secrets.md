# Configuration and secrets

`kubernetes/config/` holds local infrastructure configuration. It is separate
from the repository-root `.env` used for local application development. Cluster
manifests and Ansible read infrastructure inputs from this directory; operators
should not copy them back into scattered project folders.

## Directory contract

```text
kubernetes/config/
├── env/
│   ├── ansible/
│   ├── infrastructure/
│   ├── observability/
│   ├── production/
│   └── staging/
├── kubeconfigs/
│   ├── github-actions/
│   ├── k8s0/
│   └── k8s1/
├── credentials/
│   ├── github-actions/
│   ├── k3s/
│   └── providers/
├── generated/
└── scripts/
```

The tracked and private files follow different rules:

| File class | Tracked | Use |
| --- | --- | --- |
| `*.config.env` | Yes | Non-secret service settings used by Kustomize ConfigMap generators |
| `*.env.example` | Yes | Required variable names and safe placeholders |
| live `*.env` | No | Plaintext Kubernetes Secret inputs and Ansible/provider configuration |
| `kubeconfigs/**` | No | Administrator and namespace deployment credentials |
| `credentials/**` | No | K3s tokens, provider tokens, SSH deployment keys, recovery exports |
| `generated/**` | No | Downloaded charts and rendered manifests that can contain Secret references or rendered data |

`kubernetes/config/.gitignore` enforces these rules. Never use `git add -f` in
this directory. Store the full ignored tree in the approved encrypted 1Password
backup because Git cannot restore it.

## Environment inputs and generated objects

Each environment overlay includes `kubernetes/config/env/ENVIRONMENT` as a
Kustomize resource. The local env files generate fixed-name Kubernetes objects:

| Input | Kubernetes object | Consumers |
| --- | --- | --- |
| `backend.config.env` | ConfigMap `backend-config` | API, worker, migration init containers |
| `backend.env` and `qdrant.env` | Secret `backend-secrets` | API and worker |
| `postgres.env` | Secret `backend-postgres-app` | CloudNativePG bootstrap, API, worker, migrations |
| `redis.env` | Secret `backend-redis-auth` | Redis, API, worker, Redis backup |
| `qdrant.env` | Secret `qdrant-auth` | Qdrant, backend, Qdrant backup |
| `nango.config.env` | ConfigMap `nango-config` | Nango |
| `nango-runtime.env` | Secret `nango-runtime-secrets` | Nango |
| `nango-backend.env` | Secret `nango-backend-credentials` | API and worker |
| `nango-postgres.env` | Secret `nango-postgres-app` | CloudNativePG bootstrap and Nango |
| `web.config.env` | ConfigMap `web-config` | Next.js |
| `web.env` | Secret `web-secrets` | Next.js session and error reporting |
| `*-backup.env` | Service-specific S3 Secrets | CloudNativePG and backup jobs |

Production adds `microsandbox-control.config.env`,
`microsandbox-control.env`, `microsandbox-postgres.env`, and its PostgreSQL
backup input. Observability uses
`kubernetes/config/env/observability/grafana-admin.env`.

Kustomize disables name-suffix hashes for generated ConfigMaps and Secrets.
This keeps operator CRD references stable, but changing an object does not alter
a Pod template. Restart every consumer after applying changed configuration.

## Safe editing procedure

Set restrictive permissions before editing private files:

```sh
chmod 700 kubernetes/config
find kubernetes/config/env -type f -name '*.env' -exec chmod 600 {} +
find kubernetes/config/kubeconfigs kubernetes/config/credentials \
  -type f -exec chmod 600 {} +
```

Copy an example only when its live peer does not exist. Existing values may
decrypt database rows, sign sessions, authenticate webhooks, or preserve OAuth
connections; replacing one with a random value can make stored data unusable.

After any edit, run the repository validator:

```sh
kubernetes/config/validate.sh
```

It checks required files, env syntax, duplicate names, private file modes,
ignore coverage, accidental tracking, and removal of retired secret locations.
It does not confirm that a credential works against its remote provider.

Render into a mode-`0600` temporary file. Sending `kubectl kustomize` directly
to the terminal prints base64 Secret data:

```sh
umask 077
rendered="$(mktemp)"
kubectl kustomize kubernetes/environments/staging >"$rendered"
kubectl apply --dry-run=client -f "$rendered"
rm -f "$rendered"
```

Repeat for production before a production apply. Do not upload a rendered file
to CI artifacts, paste it into an issue, or leave it in `/tmp`.

## Apply and restart consumers

Apply only the intended environment:

```sh
kubectl apply -k kubernetes/environments/staging
# or
kubectl apply -k kubernetes/environments/production
```

Then restart the workloads that read the changed object. Examples:

```sh
kubectl rollout restart -n staging deployment/backend-api deployment/backend-worker
kubectl rollout restart -n staging deployment/web
kubectl rollout restart -n staging deployment/nango

kubectl rollout restart -n production deployment/microsandbox-control
kubectl rollout restart -n production deployment/microsandbox-preview-cache
```

Operator-managed databases may need a different rotation sequence. Changing
the bootstrap Secret does not automatically change an existing PostgreSQL role
password. Redis and Qdrant credentials also require coordinated server and
client restarts. Check the data-service document before rotating those values.

Because a full overlay apply also reconciles image fields, compare its rendered
digests with live Deployments first:

```sh
kubectl get deployment -n staging backend-api backend-worker web \
  -o custom-columns='NAME:.metadata.name,IMAGE:.spec.template.spec.containers[0].image'
rg -n 'digest:' kubernetes/environments/staging/kustomization.yaml
```

Update stale overlay digests before applying unrelated configuration, or the
apply can undo the latest CI image deployment.

## Generate or refresh service inputs

Repository scripts assemble environment files without committing them:

```sh
kubernetes/environments/generate-data-secrets.sh
kubernetes/environments/generate-backend-secrets.sh --refresh
kubernetes/environments/generate-nango-secrets.sh
kubernetes/environments/generate-web-secrets.sh
```

Use `generate-data-secrets.sh --refresh-backups` when only bucket credentials
changed. Staging has two focused checks:

```sh
kubernetes/config/scripts/generate-staging-qdrant-secret.sh
kubernetes/config/scripts/validate-staging-secrets.sh
```

The staging validator rejects missing feature credentials and a live Paystack
secret. Staging deliberately uses a Paystack test key. Model-provider keys
remain local test inputs and do not belong in live backend Secrets.

Read each script before running it against files that already exist. Some
generators refuse overwrite; others have an explicit refresh mode. Keep a
1Password snapshot before rotating continuity secrets.

## Ansible inputs

Runner deployment reads:

```text
kubernetes/config/env/ansible/runners.env
```

Its adjacent example lists the control-plane URL and API token, runner join and
API tokens, preview cache settings, private Zot registry, and optional Sentry
DSN. Ansible loads the file through
`ansible/inventory/group_vars/runners.yml`; no runner env should live under the
`ansible/` directory.

K3s installation exports administrator kubeconfigs and node tokens back into
`kubernetes/config/`. Provider credentials used by infrastructure work live in
`kubernetes/config/credentials/providers/`. Ansible does not apply Kubernetes
application Secrets or ConfigMaps.

## GitHub Actions credentials

Local deployment credentials live here:

```text
kubernetes/config/kubeconfigs/github-actions/staging.yaml
kubernetes/config/kubeconfigs/github-actions/production.yaml
kubernetes/config/credentials/github-actions/staging
kubernetes/config/credentials/github-actions/production
```

GitHub cannot read ignored local files. Base64-encode the matching kubeconfig,
private SSH key, and pinned `known_hosts` file into the `staging` or
`production` GitHub Environment. Never put an administrator kubeconfig in CI;
the namespace ServiceAccount already has the exact image-patch permissions the
workflow needs.

When rotating a deployment SSH key, update all K3s server accounts through
Ansible before replacing the GitHub secret. When rotating the ServiceAccount
token, update the local kubeconfig and GitHub secret together. Test staging
first because the production account cannot patch staging and vice versa.

## Rotation checklist

For an application credential:

1. Back up `kubernetes/config/` to 1Password.
2. Identify every producer and consumer of the value with `rg` against examples,
   generators, Kustomizations, and manifests; do not search live value text.
3. Update the ignored env file with mode `0600`, validate, and render privately.
4. Apply the affected object, restart its consumers, and wait for readiness.
5. Confirm authentication or a real non-destructive request before revoking the
   old credential.

Database passwords, encryption keys, JWT/RSA signing material, session secrets,
OAuth credentials, runner join secrets, webhook signing secrets, S3 access
keys, and certificate-provider tokens each have different external consumers.
Never rotate them as one bulk edit.

## Audit for accidental exposure

Run these checks before committing infrastructure changes:

```sh
kubernetes/config/validate.sh
git status --short
git diff --check
git ls-files kubernetes/config
```

The final command should show committed examples, `*.config.env` files,
Kustomizations, scripts, `.gitignore`, and documentation only. It must not list
live `*.env`, kubeconfigs, provider tokens, K3s tokens, private keys, or rendered
manifests.

If a secret ever reaches Git history, removing the file in a later commit is not
enough. Revoke or rotate the credential first, then clean history as a separate
incident response step.
