# Global Triggers

Global triggers are one-click installable trigger templates. They describe a
provider-backed workflow a user can add to an org; installation should create
the concrete `agent_triggers` row bound to an org, agent, and connection.

Each trigger folder contains:

- `trigger.json`: install metadata, required integration, trigger type, event
  keys, default conditions, and instruction file path.
- `instructions.md`: the task instructions copied into the installed trigger.

Templates in this folder should not require users to bring or paste arbitrary
webhook URLs. The provider event catalog stays in
`internal/mcp/catalog/providers/*.triggers.json`; these templates are
productized recipes built on top of that catalog.
