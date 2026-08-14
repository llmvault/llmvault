# Desktop app

Status: proposed
Build in: phases 2 and 4
Teams: Desktop, Runtime, Security, Agent Platform

## Its job

Desktop lets Hivy work with local files, browsers, apps, terminals, meetings, and hardware. Cloud agents can't reach those things safely on their own.

This will be Hivy's most powerful surface, so it also needs the clearest user control.

## Enrolling a computer

Pair the app to a user and org with a short-lived code or QR flow. Store the private key in the OS keychain. The server tracks the public key, owner, OS, app version, security posture, last seen time, and revocation state.

Admins may require disk encryption, screen lock, a supported OS, a recent Hivy version, device management, or an approved network. Users can see and revoke their devices. Admin “wipe” removes Hivy keys and managed caches; it must not claim to erase files outside Hivy's storage.

## Selected folders

A user grants a named folder and chooses read, create, edit, move, and delete rights. The screen shows the real path, agent or team, expiry, remote-use setting, and whether file content may leave the computer.

The runtime gives the agent an opaque folder handle, never a general filesystem root. Every operation checks the handle, resolves symlinks, blocks path escape, checks current policy, and records the agent version.

Hivy can search, read, create, edit, rename, sort, compare, watch, and save files. Bulk changes show a file list and diff. Permanent deletion is a separate high-risk action. Revocation takes effect at once.

## Controlling apps

Screen control uses screenshots, mouse, and keyboard for software without a safe API. Each app has `ask`, `allow`, or `block`; company rules can only make the user's choice stricter.

Before work starts, show the plan and target apps. While Hivy controls the computer, keep a visible border or overlay with the agent name, pause, takeover, and stop controls.

Passwords, authentication changes, money, external messages, deletion, system settings, and production work must pause for approval. If a connector can do the same job with a typed action, use it first.

Capture only the required windows where the OS allows it. Mask blocked apps, password managers, private windows, and notifications. Clipboard read and write need their own permissions.

## Shared browser

Use a dedicated Hivy browser profile or a tab the user explicitly shares. Don't inherit the entire personal browser by default.

The user sees each click and can take over for login, CAPTCHA, consent, payment, or private data, then hand control back. Site rules cover domains, downloads, uploads, cookies, clipboard, microphone, camera, and location.

Treat web content as hostile. Quarantine downloads before opening them. The run record names visited domains, transferred files, and periods of human control.

## Terminal and repositories

A terminal profile defines the folder, shell, allowed command classes, network destinations, secret sources, time limit, and whether elevation is blocked.

The agent checks the worktree first, preserves user changes, shows a plan, streams output, and produces a diff. Publishing packages, touching production, changing credentials, and running destructive commands need separate action policy.

Support worktrees, tests, builds, and collected outputs. Don't reduce terminal access to one dangerous on/off switch.

## Meetings

Recording always has a visible indicator and follows company consent rules. Users select microphone, system audio, or both. Hivy creates a transcript, timestamps, notes, decisions, and tasks.

Email, CRM updates, tickets, and calendar events remain drafts until policy allows them. Audio and transcript may have different retention dates. The first version can record locally; it doesn't need a meeting bot.

## Quick capture

A shortcut opens a small box with optional selected text, current file, clipboard, screen region, window title, URL, or voice. Each included item appears as a removable chip.

The user can ask a question, start work, choose an agent, or open items waiting for them. Installing Hivy never turns on continuous screen capture.

## Teaching mode

Teaching mode records a bounded demonstration so Hivy can draft a reusable routine. Before recording, the user selects allowed apps, windows, folders, browser domains, and whether voice explanation is included. A persistent indicator shows what Hivy is observing.

The user can pause, mark a step as optional, explain a choice, correct the agent, or discard the session. Hivy masks passwords and authentication prompts, excludes blocked windows, and stores references to credentials instead of captured values.

After the job ends, desktop uploads only the allowed event stream and approved screenshots or files. The [agent rules](07-agent-governance-spec.md) turn that material into a draft routine; the recording itself never becomes an auto-run macro.

## Local-only work

Local-only mode keeps messages, file content, screenshots, embeddings, and logs on the device. The cloud sees only the coordination fields allowed by policy. The UI must state exactly what syncs and which remote features stop working.

Models run locally or through an approved private route. Local backups are encrypted and user-controlled. Revoking the device stops cloud coordination, but it can't erase unmanaged copies.

## Remote dispatch

Web or mobile can send work to an enrolled computer. The request names the device, agent, needed folders/apps, data movement, and risk. Policy may start low-risk work, ask locally, or wait for unlock.

The remote client sees progress, questions, approvals, files, and completion. It doesn't see a live screen unless the local user starts a clearly marked sharing session.

Offline computers leave the step waiting. Locking the screen pauses visual control. Remote cancel stops future steps and reconciles any write already in flight.

## Permission center

List every device, agent, folder, app, browser domain, terminal profile, microphone, screen, clipboard, and remote right with recent use. Users can choose ask every time, allow once, allow for an agent and time window, or block. Company policy can remove the longer-lived options.

## Requirements

| ID | Hivy must |
|---|---|
| **DESK-001** | Enroll and revoke devices with OS-keystore keys. |
| **DESK-002** | Use path-scoped folder handles that block escape. |
| **DESK-003** | Preview bulk file writes and treat deletion separately. |
| **DESK-004** | Control only allowed apps with a visible stop control. |
| **DESK-005** | Provide a visible browser with site and file-transfer rules. |
| **DESK-006** | Let users take over and resume without losing progress. |
| **DESK-007** | Use bounded terminal profiles and preserve repository changes. |
| **DESK-008** | Record meetings only with clear indication and retention controls. |
| **DESK-009** | Make every captured context item visible and removable. |
| **DESK-010** | State the exact boundary of local-only work. |
| **DESK-011** | Accept policy-controlled remote work and cancellation. |
| **DESK-012** | Show and revoke every active desktop permission. |
| **DESK-013** | Record bounded teaching sessions without capturing secrets or blocked context. |

## Done when

- Folder tricks can't reach a sibling path.
- The user can spot and stop computer control in one action.
- Hostile screen text can't open a blocked app or domain.
- Remote dispatch can't widen local permissions.
- Login and system prompts stop for the user.
- Restarting the app resumes from saved progress.
- Audit separates agent control, human takeover, and approved action.

Measure device health, folder success, path attacks, browser/app completion, takeover, emergency stop, remote completion, injection warnings, terminal recovery, and meeting-result acceptance.
