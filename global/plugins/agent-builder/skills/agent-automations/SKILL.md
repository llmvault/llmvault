# Automations & triggers

Once an agent exists, you can make it run **automatically** — on a schedule, or
when an external system calls a URL. You have two tools:

- **`cron`** — recurring scheduled runs ("every day at 8am", "every hour",
  "every Monday").
- **`create_http_trigger`** — an HTTP endpoint that runs the agent whenever it's
  POSTed to (webhooks, external systems).

Both simply run the agent with a **task you write**. Crucial: once an automation
fires, you have no further control — the agent does exactly what the task says,
using its own tools and skills. So **whatever should happen (including posting
the result to Slack) must be written into the task.**

## Before you automate

- The **agent must already exist** — build it first (agent-builder skill).
- The agent must have the **tools/plugins its task needs**. To read or post in
  Slack it needs the **Slack plugin installed and connected**. Check with
  `list_org_plugins`; if it's missing (or shows `missing_requirements`), share
  the plugin's `install_url` and ask the user to install/connect it before you
  set up the automation.
- **Confirm the plan with the user** before creating anything.

## Scheduled runs — the `cron` tool

Call `cron` with `action:"create"`:

- `agent_id` — the agent to run (the one you built).
- `task_prompt` — exactly what the agent should do each run, **including how to
  deliver the output** (see "Delivering output").
- `cron_expression` — a standard 5-field cron string, **or** `interval_seconds`
  for a simple repeating interval (use one, not both).
- `channel_id` *(optional)* — the **Hivy channel UUID** the run's conversation
  lives in. This is never a Slack/provider channel id — see "Channels" below.
  Omit it and the run lands in the org's private **system** channel.
- `repeat_count` *(optional)* — stop after N runs.

Manage existing jobs with the same tool: `action:"list" | "update" | "pause" |
"resume" | "cancel"` (pass `agent_id` + `job_id`).

### Timezones — always convert to UTC first

**Every schedule runs in UTC. There is no timezone setting.** So:

1. **Confirm the timezone** the user stated the time in — *"You said 8am — which
   timezone is that?"*
2. **Convert that local time to UTC yourself**, then put the UTC time in the cron
   expression.
3. Tell the user you scheduled it for the UTC-equivalent, and that daylight-saving
   changes aren't auto-adjusted — offer to update it if their offset shifts.

Example: 8am in America/New_York (UTC−5) → **13:00 UTC** → `0 13 * * *`.

### Cron quick reference

Fields: `minute hour day-of-month month day-of-week` (all UTC).

- `0 8 * * *` — every day at 08:00
- `0 * * * *` — every hour
- `*/15 * * * *` — every 15 minutes
- `0 9 * * 1` — every Monday at 09:00
- `30 13 1 * *` — 13:30 on the 1st of each month

## HTTP triggers — the `create_http_trigger` tool

Use this when an external system should run the agent on demand:

- `agent_id` — the agent to run.
- `instructions` — what the agent should do each time it's called.
- `channel_id` *(optional)* — the run's conversation channel: a **Hivy channel
  UUID** (see "Channels" below). Omitted → the org's system channel.
- `secret` *(optional, recommended)* — a shared secret the caller must send.

It returns a **`url`** — give it to the user. They (or their system) POST to it
to fire the agent. If you set a secret, share it too and tell them to send it as
`Authorization: Bearer <secret>`.

## Channels — Hivy IDs, never Slack IDs

Two different things share the word "channel". Do not mix them up:

- A **Hivy channel** is where an automation's conversation lives. Its id is a
  **UUID** (`3f2a…`). This — and only this — is what `channel_id` accepts.
- A **Slack channel id** looks like `C0XXXXXXX`. It is never a valid
  `channel_id`. In Slack-triggered sessions your inbound context contains a
  `slack_channel_id:` line — that is the Slack id, not a Hivy channel id.

To find the right Hivy channel UUID, call **`list_channels`** (no arguments):

```json
{}
```

It returns every channel you can schedule into: `id` (the UUID to use), `name`,
`kind`, `is_default` (the org's #general), `is_system`, and — for channels
linked to an external app — `external_provider`, `external_resource_name`, and
`external_resource_key` (the provider's own id, e.g. the Slack `C0XXXXXXX`).
That last field is how you translate: when the user says "this channel" in a
Slack conversation, match your context's `slack_channel_id` against
`external_resource_key` and use that channel's `id`.

When the user names no channel, **omit `channel_id`** — the run lands in the
org's private **system** channel (auto-created for every org). Say so in your
recap so the user knows where to find the run history.

## Delivering output (e.g. posting to Slack)

Automations do **not** post the agent's output anywhere on their own. If the
user wants the result in a Slack channel, the `task_prompt` / `instructions`
must tell the agent to do it — and the agent must have the Slack plugin. Write it
explicitly, for example:

> "Summarize the last 24 hours of messages in the **#general** Slack channel.
> Then load the `slack` skill and post the summary to **#general**."

So: name the exact channel, and instruct the agent to **load the `slack` skill
and post**. If the Slack plugin isn't installed on that agent, stop and get the
user to install it first (share the `install_url`).

## Full example — "every day at 8am, summarize #general and post it there"

1. Make sure the agent exists and has the Slack plugin (install it via the shared
   `install_url` if not).
2. Ask the timezone: user says 8am ET → **13:00 UTC**.
3. `cron` create:
   - `agent_id`: the agent
   - `cron_expression`: `0 13 * * *`
   - `task_prompt`: *"Read the last 24 hours of messages in the #general Slack
     channel and write a concise summary of what happened. Then load the `slack`
     skill and post the summary to #general."*
4. Verify the tool returned a job with a `next_run_at`; tell the user it's set
   (mention it runs at 13:00 UTC = their 8am) and how to change or pause it.

## Verify every action

After each `cron` or `create_http_trigger` call, check the result before telling
the user it's done:

- A schedule should come back with a `job_id` and a `next_run_at`.
- An HTTP trigger should come back with a `url`.
- If a call returns an error, read it, fix the input, and retry. Never say an
  automation is set up until the tool confirmed it.

## Don't do these

- Scheduling before the agent exists, or before it has the plugins its task needs.
- Forgetting the timezone — always confirm and convert to UTC.
- Writing a task that produces a result but never delivers it (there is no
  automatic output — the task must post/send it).
- Guessing an `agent_id`, `channel_id`, or cron syntax — `list_agents` and
  `list_channels` exist so you never have to.
- Passing a Slack `C0XXXXXXX` id (or anything from a `slack_channel_id:` context
  line) as `channel_id` — it will be rejected; translate it via `list_channels`
  (`external_resource_key`) first.
- Sharing an HTTP trigger URL with no secret when it should be protected.

## When something goes wrong

- **`cron` rejects the cron_expression** — it's malformed; fix the 5-field
  syntax and retry.
- **"agent not found"** — wrong `agent_id`; call `list_agents` to find it.
- **"channel_id must be a uuid"** — you passed a Slack/provider id. Call
  `list_channels` and use the channel's `id` (match Slack ids against
  `external_resource_key`).
- **"channel_id not found" / "agent is not available in this channel"** — the
  UUID is from another org, archived, or the agent isn't allowed there;
  `list_channels` shows only valid choices.
- **The scheduled run happens but nothing shows up in Slack** — the task didn't
  instruct the agent to post, or the agent lacks the Slack plugin. Update the
  job's `task_prompt` (`cron` `action:"update"`) and/or get the Slack plugin
  installed.
- **It ran at the wrong time** — the cron was set in the wrong timezone.
  Recompute the UTC time and update the job.
- **User wants to stop or change it** — use `cron` `action:"pause" | "resume" |
  "cancel" | "update"` with the `agent_id` and `job_id`.
