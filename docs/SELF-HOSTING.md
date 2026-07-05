# Self-Hosting — Domain & Email Configuration

This reference covers the environment variables that let you run Hivy under **your own domain** and send email through **your own provider**. Everything below is pure configuration — no code changes, no rebuilds. A single prebuilt image is repointed entirely via env.

> **Scope.** This document covers the *application* config (main app, API, frontend, and the Docker sandbox provider). The full deployment stack — reverse proxy / TLS, wildcard preview DNS, and orchestration — is a separate concern and is **not** covered here. The target topology is a single VPS running the backend with the **Docker sandbox provider**, provisioning sandbox containers on the same host.

---

## Quick start

Most operators set a handful of values. Copy `.env.example`, then set at minimum:

```sh
# --- API / backend ---
HIVY_FRONTEND_URL=https://app.yourdomain.com        # required
HIVY_PREVIEW_BASE_DOMAIN=preview.yourdomain.com     # required (see "Sandbox previews")
HIVY_API_WEBHOOK_BASE_URL=https://api.yourdomain.com
HIVY_AUTH_AUDIENCE=https://api.yourdomain.com

# --- Frontend (web service) ---
HIVY_PUBLIC_API_URL=https://api.yourdomain.com       # browser-facing API URL
HIVY_API_URL=http://api:8080                          # server→API (internal ok)
HIVY_CONNECTIONS_HOST=connect.yourdomain.com
HIVY_ASSETS_URL=https://assets.yourdomain.com

# --- Email (optional; omit for local dev, see "Email") ---
HIVY_SMTP_HOST=smtp.yourprovider.com
HIVY_SMTP_PORT=587
HIVY_SMTP_USERNAME=...
HIVY_SMTP_PASSWORD=...
HIVY_EMAIL_FROM=Your Name <hello@yourdomain.com>
```

The backend **fails fast at startup** if a required var is missing, so a misconfiguration is loud rather than silent.

---

## Backend (API + worker)

| Var | Required | Default | Purpose |
|---|---|---|---|
| `HIVY_FRONTEND_URL` | ✅ | — | Public URL of the web app. Auto-added to CORS. |
| `HIVY_PREVIEW_BASE_DOMAIN` | ✅ | — | Wildcard host suffix for sandbox previews (`{port}-{id}.<domain>`). Injected into the agent system prompt + app preview URLs. |
| `HIVY_API_WEBHOOK_BASE_URL` | — | `https://api.usehivy.com` | Public API base for provider webhooks, triggers, and the runtime control plane the sandboxes call back to. Set to your API URL. |
| `HIVY_AUTH_AUDIENCE` | — | `https://api.usehivy.com` | JWT audience claim. |
| `HIVY_PROXY_HOST` | — | `proxy.usehivy.com` | LLM proxy host. Point at the hosted proxy or run your own. |
| `HIVY_CORS_ORIGINS` | — | (frontend URL added automatically) | Extra allowed CORS origins (comma-separated). |
| `HIVY_PREVIEW_CNAME_TARGET` | — | `preview-proxy.usehivy.com` | Advanced: custom per-customer preview domains. Inert without the preview stack. |

> These callback URLs are **infrastructure values** you set per deployment (e.g. `http://api:8080` inside a Docker network, `https://api.yourdomain.com` publicly). They are not derived — set them explicitly.

---

## Frontend (web service)

The frontend reads these at **runtime** (injected into the page as `window.__HIVY_ENV__`), so the same prebuilt image works under any domain. Set them on the web container.

| Var | Required | Default | Purpose |
|---|---|---|---|
| `HIVY_PUBLIC_API_URL` | ✅¹ | falls back to `HIVY_API_URL` | Browser-facing API URL (OAuth redirects, SSE). Must be publicly reachable. |
| `HIVY_API_URL` | ✅¹ | — | Server→API URL used by the `/api/proxy` route. May be an internal address. |
| `HIVY_CONNECTIONS_HOST` | ✅ | — | Host serving integration connect flows + template logos. |
| `HIVY_ASSETS_URL` | ✅ | — | Base URL of your public assets host (images, uploads). |
| `HIVY_PREVIEW_BASE_DOMAIN` | ✅ | — | Same preview suffix as the backend — used to recognize preview URLs in chat. |
| `HIVY_ADMIN_ENABLED` | — | `false` | Set `true` to expose the admin panel. |

¹ At least one of `HIVY_PUBLIC_API_URL` / `HIVY_API_URL` must be set; the browser value defaults to the latter when the former is omitted.

---

## Sandbox previews

User-facing sandbox previews are served at `https://{port}-{sandbox_id}.<HIVY_PREVIEW_BASE_DOMAIN>`. Set `HIVY_PREVIEW_BASE_DOMAIN` on **both** the backend and the web service to the same value (e.g. `preview.yourdomain.com`).

Serving those hostnames requires wildcard DNS (`*.preview.yourdomain.com`) and TLS — that plumbing is part of the deployment stack (separate) and not configured here.

---

## Email

Email is **provider-agnostic over SMTP**. Point at any SMTP server (Resend SMTP, Amazon SES, Postmark, or a self-hosted relay).

| Var | Required | Default | Purpose |
|---|---|---|---|
| `HIVY_SMTP_HOST` | — | (empty) | SMTP server host. **When empty, emails are rendered and written to a temp file (path logged) instead of sent** — ideal for local dev. |
| `HIVY_SMTP_PORT` | — | `587` | SMTP port. |
| `HIVY_SMTP_USERNAME` | — | (empty) | SMTP auth username. Omit for unauthenticated relays. |
| `HIVY_SMTP_PASSWORD` | — | (empty) | SMTP auth password. |
| `HIVY_SMTP_TLS` | — | `starttls` | `starttls`, `ssl` (implicit TLS, e.g. port 465), or `none`. |
| `HIVY_EMAIL_FROM` | ✅² | — | From header, e.g. `Acme <hello@acme.com>`. Use a domain you've verified (SPF/DKIM) with your provider. |
| `HIVY_EMAIL_SITE_URL` | — | — | Substituted into email footer links (`{{{siteUrl}}}`). |
| `HIVY_EMAIL_ASSET_URL` | — | — | Base URL for the email logo image (`{{{assetBaseUrl}}}/hivy-logo.png`). |

² Required for actual SMTP delivery; not needed when using the temp-file fallback.

**How it works:** email is sent asynchronously. Handlers enqueue a job; the worker renders the template (React Email → prebuilt HTML with `{{{placeholders}}}`, substituted in Go) and delivers it over SMTP — or writes it to a temp file if SMTP is unconfigured.

**Editing templates:** sources live in `internal/email/templates/`. After editing, run `make emails` to regenerate the embedded HTML in `internal/email/templates/dist/`.

---

## Sandbox provider

For a single-VPS self-host, use the Docker sandbox provider (provisions containers on the same host):

```sh
HIVY_SANDBOX_PROVIDER_ID=docker
HIVY_SANDBOX_DOCKER_HOST=unix:///var/run/docker.sock
```

Sandbox runtime/app images are pulled from the official `ghcr.io/usehivy/*` registry; the image **tag** is configurable via `HIVY_SANDBOXES_RUNTIME_IMAGE_TAG` / `HIVY_SANDBOXES_APP_IMAGE_TAG`.
