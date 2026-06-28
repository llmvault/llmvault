# Global Triggers

Global triggers are installable provider automation templates. They describe a
fixed provider event plus seed values the installation UI can prefill before it
creates the concrete trigger row for an org, agent, channel, and connection.

Each trigger folder contains:

- `trigger.json`: template metadata, required integration, fixed
  `trigger.key`, and `trigger.defaults`.
- `instructions.md`: default instructions loaded into
  `trigger.defaults.instructions`.

The trigger key is not user-editable. Defaults are only starting values:
installation should collect the actual trigger value and instructions before
creating the installed trigger.

Provider-specific delivery code owns event handling. Do not add generic webhook
conditions or provider event catalogs here.
