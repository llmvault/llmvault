# Authoring a Hivy skill

This reference explains how to make a skill useful after publication. The publishing tools already describe their arguments, validation rules, and result shapes.

## Understand the delivery model

A custom skill lives inside a plugin owned by one team. Once published, it appears in that team's agents' skill inventory. An agent initially sees the skill's name and trigger description; it loads the full instructions and supporting files only when the skill is relevant.

This makes the trigger description a routing decision and the body an execution guide. Write each for that distinct job.

## Write trigger text that supports a decision

Name the concrete situations, verbs, artifacts, or systems that indicate the skill is relevant. Include important near-boundaries when they help an agent distinguish this skill from a related one.

Weak: `Helps with sales proposals.`

Useful: `Use when drafting, reviewing, or formatting a sales proposal or quote using the team's pricing table and approval process.`

Do not summarize the entire workflow in the trigger. Its purpose is to help the agent decide whether to load the skill.

## Write the body as an operating guide

Lead with the outcome the skill guarantees, then describe the workflow in the order an agent should execute it. Include:

- the information to gather before acting;
- decision points and why they matter;
- concrete commands, templates, or examples specific to the team;
- validation checks and recognizable completion criteria;
- failure handling where recovery is possible;
- clear escalation when user input or unavailable access is required.

Avoid explaining general concepts the agent already understands. Preserve the details that would otherwise require tribal knowledge, repeated discovery, or guesswork.

## Use supporting files deliberately

Keep the main body scannable. Put long policies, API references, examples, and style guides in `references/`; reusable output skeletons in `templates/`; executable helpers in `scripts/`; and static inputs in `assets/`.

When the body depends on a supporting file, say when to read or run it. Files materialize under `.skills/<skill-slug>/`, so instructions must use paths that resolve from there rather than paths from the authoring workspace.

## Make it portable

Write for a fresh agent sandbox, not the environment left behind while authoring. Use `references/sandbox-environment.md` to confirm available tools and persistent paths. Make setup repeatable when a dependency is unavoidable.

Reference secrets through declared org environment variables. Never embed values, ask the user to paste them into chat, or assume a private credential exists without handling the missing-configuration case.

## Verification checklist

- The trigger description distinguishes when the skill should load.
- The body teaches the workflow rather than documenting publishing tools.
- Every command and script has been exercised in the Hivy sandbox.
- Supporting paths resolve after the skill is loaded by a fresh agent.
- Required configuration is named without exposing secret values.
- The completion checks demonstrate that the workflow actually succeeded.
