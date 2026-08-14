# ChatGPT for business

## What OpenAI sells

[ChatGPT Business pricing](https://learn.chatgpt.com/docs/pricing) lists $20 per person each month on annual billing, with at least two users, or $25 on monthly billing. It includes a company workspace, desktop and mobile apps, SAML SSO, MFA, larger cloud machines, and no training on company data by default.

Enterprise adds the controls large companies ask for: SCIM, domain verification, role controls, analytics, compliance records, retention choices, regional options, and customer-managed encryption keys.

The important shift isn't a pricing feature. OpenAI now sells completed work, not only better chat.

## Work that keeps running

[OpenAI's Work guide](https://learn.chatgpt.com/docs/get-started-with-work) asks users to delegate a result such as a report, slide deck, analysis, recurring update, workflow, or file. [Long-running work](https://learn.chatgpt.com/docs/long-running-work) can pause, resume, change direction, or stop without losing the task.

[Scheduled tasks](https://learn.chatgpt.com/docs/automations) can run inside an existing conversation or as separate recurring work. They keep context, support calendar rules, and continue in the background.

What this means for Hivy: a chat message should create a durable work item. Users need state, progress, questions, actions, output, and history after they close the app.

## Browser and computer use

[OpenAI's browser](https://learn.chatgpt.com/docs/browser) has two modes. Desktop gives the user and agent a shared browser with its own profile, history, annotations, and takeover. Cloud browsing runs somewhere else and doesn't inherit the user's local signed-in state.

[Computer Use](https://learn.chatgpt.com/docs/computer-use) controls macOS and Windows apps through the screen, mouse, and keyboard. Users grant apps separately. Admins can turn it off. Sensitive or disruptive actions ask again. OpenAI recommends a connector or MCP tool when one exists, then falls back to visual control when structured access can't do the job.

Hivy needs the same order: connector first, browser second, screen control last. Every mode must still pass through one policy and approval system.

## Phone-to-computer work

[OpenAI Remote](https://learn.chatgpt.com/docs/remote) pairs a phone with a Mac or Windows computer using the same account and a QR code. The phone can start coding work, watch progress, answer questions, approve steps, inspect diffs, and review tests while the computer stays awake and online.

This establishes a clear mobile job. The phone doesn't need the full desktop app. It needs to command, approve, inspect, and intervene.

## Files and review

[The artifact viewer](https://learn.chatgpt.com/docs/artifacts-viewer) handles documents, presentations, spreadsheets, PDFs, and HTML previews. Users can point at a passage, chart, slide, or visual area and ask for a change.

Hivy's Files, Canvas, Sheets, Drive, brands, and apps should become one versioned artifact system. Review must bind to an exact file version so an agent can't change a document after approval and still send it.

## Notifications and activity

[OpenAI notifications](https://learn.chatgpt.com/docs/notifications) separate completion, permission, and question alerts. Activity views group unread work, running work, and items waiting for the user. Depending on the surface, alerts can use desktop, push, email, or SMS.

Hivy should copy the event model, not the exact UI. A notification must say what changed and open the right work item.

## Admin and compliance

[Admin setup](https://learn.chatgpt.com/docs/enterprise/admin-setup) splits workspace access, local runtime rules, cloud execution, API access, plugins, connectors, and permissions inside connected systems.

[Roles and workspace permissions](https://learn.chatgpt.com/docs/enterprise/roles-and-workspace-permissions) separate feature access from admin power. Groups can receive specific product permissions; sensitive desktop history starts off.

[Governance](https://learn.chatgpt.com/docs/enterprise/governance) covers adoption analytics, aggregate reports, credit controls, and audit records. [The Compliance API](https://learn.chatgpt.com/docs/enterprise/compliance-api) sends evidence into legal, identity, and security workflows.

## What Hivy must match

- Durable background and scheduled work.
- Shared browser, cloud browser, and guarded computer use.
- Remote desktop dispatch and cross-device approval.
- Native business files with precise review comments.
- SAML SSO and MFA on Business.
- SCIM, roles, compliance export, retention, residency, and customer keys on Enterprise.

## Where Hivy can win

ChatGPT still starts with a personal assistant model. Hivy can make the company-owned agent the main object, then tie every run to a team, policy, budget, approval chain, device, and accepted outcome.
