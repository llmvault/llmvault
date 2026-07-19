# Staging credential policy

Staging starts from the requested production database snapshot. It must retain
the production KMS, JWT, RSA, sandbox-encryption, OAuth, delivery, Nango, and
other application continuity secrets so imported encrypted rows and sessions
remain usable.

The exceptions are deliberately environment-specific:

- Paystack uses an `sk_test_...` key;
- application uploads use the `usehivy-staging-app` Hetzner bucket and its own
  access key;
- PostgreSQL, Redis, Qdrant, Nango PostgreSQL, and backup credentials are
  generated separately inside the staging namespace;
- public callback/base URLs use the `staging.*.usehivy.com` hosts.

Generate the ignored backend Secret input from Railway, the live Microsandbox
control Secret, `.env.hetzner-s3`, the root staging Paystack test key, and any
new values supplied in `/tmp/hivy-backend-new.env`:

```sh
chmod 600 /tmp/hivy-backend-new.env
kubernetes/environments/generate-backend-secrets.sh --refresh
```

The script verifies that Railway API and worker continuity secrets agree,
refuses a live Paystack key in staging, and writes the result with mode `0600`.
The plaintext files under `kubernetes/environments/*/secrets/*.env` are ignored
by Git.

Model-provider keys remain excluded from live deployment. They are only used
for local and end-to-end testing.

The web application has its own ignored Secret input. Generate it after the
backend continuity secrets; the script copies production observability values
but creates a separate staging session secret:

```sh
kubernetes/environments/generate-web-secrets.sh
```
