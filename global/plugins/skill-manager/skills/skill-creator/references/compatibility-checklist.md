# Compatibility checklist — porting a skill to Hivy

Run every item against the source skill (or your draft). Each finding is **works / adapt / incompatible**; adapt or drop before publishing, and list every adaptation and drop in your approval report to the user.

## 1. Browser & web automation

- Playwright / Puppeteer / Selenium / "install Chrome" instructions → **adapt**: rewrite to the preinstalled `browser` CLI (headless Chromium). Verify each rewritten command by running it.
- Instructions to "open a browser and click…" for a human → **adapt** to `browser` automation or reframe as a link you share with the user.
- Anything needing a visible window, extensions, or a real user profile → **incompatible**.

## 2. Dependencies & installation

- `apt-get install`, `brew install`, `npm i -g`, `pip install` as one-time setup → **adapt**: these do not persist and other agents' sandboxes never had them. Options, in order of preference: use a preinstalled equivalent; vendor a small script into the skill's `scripts/`; make installation an explicit repeatable step scoped to the project directory (`npm install` in-project, `python3 -m venv /workspace/...`).
- `pip` at all on the standard image → **adapt**: no pip is preinstalled; prefer `python3` stdlib, `node`, or preinstalled CLIs (jq, pandoc, ffmpeg, imagemagick, sqlite3, tesseract, poppler).
- Docker, docker-compose, devcontainers → **incompatible** on the standard image. Say so; offer a non-container redesign if one exists.

## 3. Secrets & configuration

- Hardcoded API keys, tokens, `.env` files shipped in the repo → **adapt**: strip the values; declare injected names in `required_environment_variables`; instruct reading `$HIVY_ORG_<NAME>`; tell the user which variables to add in workspace settings (share the `environment_settings_url` from the tool response).
- Instructions to paste a secret into a config file or chat → **adapt** to org env vars. Never reproduce a secret value anywhere.
- OAuth flows requiring a localhost callback and human browser login → usually **incompatible**; prefer API-key auth via env vars, or the org's existing platform connections (GitHub, Slack, etc.).

## 4. Filesystem & persistence

- Paths under `~`, `~/.config`, `/opt`, `/usr/local` → **adapt** to `/workspace` (note `HOME=/workspace`) or skill-relative paths under `.skills/<slug>/`.
- "Cache/remember between runs" assumptions → **adapt**: only `/workspace` persists, and only within one sandbox — cross-agent or cross-sandbox state needs memory tools or an external store, not files.
- Absolute paths into the source repo (git mode) → **adapt**: the reading agent does not have the repo; ship needed files in the skill's `files` or instruct the clone explicitly.

## 5. Processes, ports, scheduling

- Long-running commands (builds, batch jobs > ~2 minutes) → **adapt**: instruct background execution with status polling; never rely on a single blocking call.
- Servers meant to be reached externally → **adapt**: registered preview ports only, URL scheme `https://{port}-{sandbox_id}.preview.usehivy.com`; recommend `hivy-guardian` for resilience; note the ~5-minute idle sleep.
- cron / systemd / launchd scheduling → **incompatible** inside the sandbox; scheduling belongs to platform triggers, not the skill. While systemd is available inside sandboxes, this is not the way to create schedules or repeatable tasks. Should use hivy platform native tools instead.

## 6. Format & metadata (git mode)

- Source frontmatter fields like `allowed-tools`, `license`, `metadata`, `version` → **adapt**: not modeled on Hivy. Fold anything meaningful into the content or tags; drop the rest and note it.
- Source `description` → usually **adapt**: rewrite as Hivy trigger text ("Use when…"); upstream descriptions are often too vague to trigger on.
- Files outside `references/`, `templates/`, `scripts/`, `assets/` → **adapt**: relocate under an allowed directory and fix every path mention in the content.
- Skills that reference other skills or a marketplace → **adapt**: name the Hivy skill/plugin equivalents or inline what is needed.

## 7. Verification gate (before the approval report)

- [ ] Every script in `files` ran successfully in your sandbox.
- [ ] Every CLI the content instructs exists here (`command -v <tool>`) or has an explicit in-workflow install step.
- [ ] Every env var the content reads is declared in `required_environment_variables`, and no secret value appears anywhere.
- [ ] Every path in the content resolves for a *fresh* agent that just ran `skill_view` (`.skills/<slug>/...` or `/workspace/...`).
- [ ] Anything you could not test (missing credentials, org-specific systems) is called out in the report as untested — not silently assumed.
