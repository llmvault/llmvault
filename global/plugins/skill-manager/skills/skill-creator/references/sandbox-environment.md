# The Hivy sandbox — what a skill can rely on

Every agent session runs in an isolated Linux (Debian) microVM. A skill's instructions execute in this environment — nothing else. Write skills against these facts, and dry-run against them before publishing.

## Preinstalled tools (safe to instruct)

- **Shell & core:** bash, zsh, coreutils, curl, wget, jq, yq, ripgrep (`rg`), fd, sqlite3, unzip/zip/tar/gzip/zstd/xz, shellcheck, shfmt, lsof, netcat, openssh-client.
- **Git:** `git` (plus git-lfs) and the `gh` GitHub CLI. Authentication to GitHub is automatic through the org's GitHub connection — no tokens to manage. Public HTTPS clones always work; private repos only if the org connection can access them.
- **Languages:** `python3` (no pip on the default image), `node` LTS with `npm`/`npx`.
- **Documents & media:** ffmpeg, imagemagick, ghostscript, pandoc, poppler-utils (pdftotext etc.), tesseract-ocr, plentiful fonts.
- **Browser:** the `browser` CLI (agent-browser, headless Chromium). This is the ONLY browser automation available — there is no display server, no GUI, and no Playwright/Puppeteer/Selenium runtime. Skills that need web interaction must instruct `browser` commands.

Anything not in this list must be treated as absent. In particular there is **no docker** on the standard image — never instruct `docker run` / `docker compose` unless the user has confirmed their agents use the developers image.

## Persistence — the most common porting mistake

- **`/workspace` persists** across the sandbox's sleep/wake cycles. `HOME` is `/workspace`. Cloned repos live in `/workspace/repos`. Project-local installs (`npm install` in a project dir, a Python venv under `/workspace`) survive within the sandbox's lifetime.
- **Everything outside `/workspace` is ephemeral.** `apt-get install`, `npm install -g`, `pip install` to system paths vanish when the sandbox is recycled — and a *different agent* loading your skill starts in a *different sandbox* that never had them.

Therefore a skill must either: (a) use only preinstalled tools, (b) ship what it needs in its own `scripts/` files, or (c) make installation an explicit, repeatable first step of the workflow (e.g. "if `npx tsx --version` fails, run `npm install -D tsx` in the project directory"). Never write "install X once and it will be there next time".

## Execution constraints

- Shell commands time out at ~120 seconds by default. Keep commands bounded; split longer workflows into independently verifiable steps.
- Command output is truncated (last ~2000 lines / 50 KB). Pipe large outputs to files under `/workspace` and inspect selectively.
- The sandbox sleeps after ~5 minutes idle and wakes on demand; long-running servers pause with it. For dev servers, `hivy-guardian` supervises and restarts a process on a port.
- Only registered preview ports (defaults include 3000, 5173, 8000, 8080; org-configurable) are reachable from outside, at a URL of the form `https://{port}-{sandbox_id}.<preview-domain>`. Never hand-build this hostname — the actual preview URL is provided by the platform tooling (e.g. `make preview` or the preview-env endpoint). A server on an unregistered port works inside the sandbox but has no public URL. Port 7080 is reserved.

## Network

Outbound network is open: public APIs, npm, PyPI, arbitrary HTTPS all work. Inbound is preview-ports only. Model/LLM access is proxied by the platform — a skill that needs to call an external LLM API directly must bring its own key via an org environment variable.

## Environment variables

Org variables set in workspace settings are injected into every sandbox as `HIVY_ORG_<NAME>`. Platform-reserved variables are prefixed `HIVY_` — skills must not instruct overwriting them. Read secrets from the environment at run time; never persist them to files that outlive the step, and never echo them into output.

## Hard incompatibilities (reject or redesign)

- GUI/desktop applications, X11, anything needing a display (headless `browser` is the only exception).
- Docker/containers on the standard image.
- System daemons, systemd units, cron inside the sandbox, sudo-requiring setup.
- Host hardware: audio/video capture devices, USB, GPUs.
- "Install once globally, use forever" workflows (see persistence above).
- Listening on arbitrary ports for external traffic (preview ports only).
