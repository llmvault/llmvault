# Claude for business

## What Anthropic sells

[Claude pricing](https://claude.com/pricing) lists Team for two to 150 people. Standard seats cost $20 per person each month on annual billing or $25 monthly. Premium seats cost $100 annually or $125 monthly and include more usage.

Team includes Claude Code, Cowork, Microsoft 365, company search, billing and admin tools, SSO, connector controls, desktop deployment, and no training on company data by default.

[Claude Enterprise](https://support.claude.com/en/articles/9797531-what-is-the-enterprise-plan) adds audit logs, SCIM, custom retention, Compliance and Analytics APIs, customer-managed keys, US-only inference, workplace connectors, BAA support, spend limits, and added usage billed at API rates. [Anthropic's enterprise page](https://claude.com/solutions/enterprise) also lists domain capture, roles, provisioning, audit, retention, and connector access.

## Cowork across devices

[Cowork on web, desktop, and mobile](https://support.claude.com/en/articles/15520349-use-claude-cowork-on-web-desktop-and-mobile) lets a user start, steer, review, and resume the same cloud task anywhere. Projects, skills, plugins, schedules, connectors, and created-file previews travel with it.

Desktop adds local folders, applications, browser access, and other machine-bound work. The cloud and local modes are related, but they're not the same storage boundary. Anthropic says some local Cowork data sits outside normal central retention and export.

Hivy should make that boundary impossible to miss. Users need to know what stayed local, what synced, and what a phone or admin can see.

## Dispatch

[Claude Dispatch](https://support.claude.com/en/articles/13947068-assign-tasks-from-anywhere-in-claude-cowork) creates a persistent thread between a phone and desktop. A mobile user can assign work that needs local files, connectors, plugins, the browser, or apps. Results can include a spreadsheet, memo, presentation, comparison, or code change. Push alerts report completion and approval requests.

This is one of Claude's strongest business workflows. Hivy needs it.

## Computer use and permission modes

[Claude computer use](https://support.claude.com/en/articles/14128542-let-claude-use-your-computer-in-cowork) chooses a connector first, browser automation next, then direct screen control. Apps have permission settings and blocklists. The system scans for prompt injection before risky computer work.

[Cowork setup](https://support.claude.com/en/articles/13345190-get-started-with-claude-cowork) offers Manual, Auto, and Skip modes. Connector tools can ask every time, stay allowed, or stay blocked.

[Team and Enterprise controls](https://support.claude.com/en/articles/13455879-use-claude-cowork-on-team-and-enterprise-plans) let an org forbid saved “always allow” settings for write tools. The stricter org or user setting wins. A tool that doesn't declare itself read-only gets an approval gate.

Hivy should use the same simple permission choices, but the server must turn them into exact rules by action, target, person, device, and amount.

## Projects, files, and extensions

[Claude projects](https://claude.com/docs/cowork/guide/projects) group persistent context and work. Plugins and skills package repeatable methods and tools.

[File creation](https://support.claude.com/en/articles/12111783-create-and-edit-files-with-claude) covers Word, PowerPoint, Excel, and PDF on web, desktop, and mobile. Users can make financial models, charts, reports, and presentations without opening a separate creation tool first.

Hivy needs solid native files and previews before it spends time on broad media features.

## Enterprise lessons

Claude exposes several buyer-friendly controls that Hivy lacks today: SCIM, retention, compliance export, analytics export, customer keys, regional inference, spend limits, and desktop deployment rules.

There is also a warning in Anthropic's own docs. Cowork doesn't yet fit every central audit and retention path. Hivy should avoid splitting local and cloud evidence into two admin worlds.

## What Hivy must match

- Cloud tasks that follow the user across web, desktop, and mobile.
- Phone-to-desktop Dispatch.
- Selected folders, apps, browser, screen control, and permission modes.
- Skills, plugins, schedules, projects, and native office files.
- Business SSO, connector controls, deployment controls, and no training by default.
- Enterprise SCIM, audit, retention, APIs, spend limits, regional policy, and customer keys.

## Where Hivy can win

Hivy can give local and cloud work one policy record, one approval system, one audit stream, and one outcome model. That is cleaner for companies than asking admins to reason about different control coverage on each surface.
