# Mobile app

Status: proposed
Build in: phases 2 to 4
Teams: Mobile, Product, Platform, Security

## Its job

Mobile is for asking, capturing, watching, approving, and dispatching. It shouldn't start as a cramped copy of the web agent builder.

Use five tabs: Work, Ask, Approvals, Artifacts, and Account. A small activity marker shows running work and items waiting for the user.

## Voice

Press-and-hold or hands-free mode records a request and shows live text. The user can edit it, choose an agent, and remove any attached context before sending.

Two-way voice can read progress and ask follow-up questions. Material actions move to a visual confirmation. Show whether raw audio uploads, stays local, or disappears after transcription; transcript retention is a separate setting.

## Camera and field work

Support photos, short video, document scans, barcodes, QR codes, and optional location. Good first uses include inspections, receipts, serial numbers, inventory, whiteboards, and damage reports.

Show the metadata that will upload. Remove blocked EXIF data and let users redact images. Offline drafts stay encrypted. Each item keeps capture time, contributor, edits, and integrity data.

Templates should warn about missing evidence before submission. A safety issue may alert the right team even if the rest of an inspection is unfinished.

## Approvals

An approval card shows the agent, work item, action, target, risk, reason, change, amount, evidence, rule, and expiry. The user can approve, deny, edit permitted fields, ask for proof, or delegate.

The card points to one payload hash. If anything changes, it becomes stale. High-risk approval invokes Face ID, Touch ID, or Android biometrics; Hivy stores only the successful step-up result.

Lock-screen text follows company sensitivity rules. “Action needs approval” may be all the phone can show.

## Watch and intervene

Work groups items into working, waiting for you, scheduled, failed, and completed. Each item shows current step, execution place, next event, cost, and blocker.

Users can answer, comment, attach evidence, pause, cancel, retry safe steps, take ownership, or send a local step to desktop. Foreground progress uses resumable events; background progress uses push. The server timeline stays authoritative.

The same view also starts approved routines and watches agent teams. A group of agents still appears as one work item with named child owners and handoffs; the phone shouldn't force users to chase several chat threads.

## Share sheet and desktop dispatch

The native share sheet accepts links, text, images, PDFs, documents, and files. Users pick an agent or project, add a request, remove attachments, and select cloud or desktop when allowed. Nothing uploads before submission.

Desktop dispatch shows which enrolled computers are online, unlocked, and eligible. It also shows required local rights and data movement. Live screen viewing needs a separate, visible desktop permission; normal work sends structured progress.

## Files on a phone

Preview documents, slides, sheets, PDFs, images, video, reports, and Hivy apps. Users can comment, annotate, accept, reject, share under policy, or make small edits. Heavy layout work belongs on web or desktop.

Offline files are encrypted and controlled by company policy. Revocation blocks protected files and removes managed caches when the app reconnects.

## Offline rules

Cache only allowed item summaries, draft comments, captures, and files. Queue permitted remote changes with an idempotency key and label them `pending sync`.

Don't complete an approval offline. The user can draft a decision, but the server must recheck identity, expiry, payload, policy, and target state.

## Device safety

Each install gets its own key. Store tokens and drafts in encrypted OS storage. Companies may require biometrics, passcode, supported OS, managed app settings, and a device that isn't rooted or jailbroken.

Support app lock, remote session revocation, push-token rotation, and managed-cache removal. Keep secrets and sensitive approval data out of analytics, crash reports, notifications, and logs.

Primary flows must work with screen readers, larger text, reduced motion, captions, and low bandwidth. Uploads resume by chunk.

## Requirements

| ID | Hivy must |
|---|---|
| **MOB-001** | Let users edit voice requests and see audio handling. |
| **MOB-002** | Capture scans, codes, media, and optional location with metadata control. |
| **MOB-003** | Show exact-payload approvals with expiry and biometric step-up. |
| **MOB-004** | Let users watch, answer, pause, cancel, retry, and take over work. |
| **MOB-005** | Use the native share sheet without early upload. |
| **MOB-006** | Send safe push alerts with signed-in deep links. |
| **MOB-007** | Send eligible work to an enrolled desktop. |
| **MOB-008** | Preview and mark up business files. |
| **MOB-009** | Queue allowed offline work with visible state and idempotency. |
| **MOB-010** | Enforce device posture, app lock, encryption, and revocation. |
| **MOB-011** | Support accessibility and slow connections. |
| **MOB-012** | Start approved routines and supervise coordinated agent teams from one timeline. |

## Done when

- Old or changed data can't receive approval.
- Locked-screen alerts don't leak forbidden fields.
- A field worker can capture offline and sync once.
- One task can start on phone, run on desktop, ask a question, receive approval, and return a result.
- Device revocation stops new calls and protected-file access.

Measure voice completion, uploads, approval time, expired requests, biometric success, push delivery, desktop dispatch, offline conflicts, file acceptance, and crash-free use.
