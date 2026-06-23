# Global Schedules

Global schedules are one-click installable schedule templates. They describe
recurring provider-backed workflows a user can add to an org; installation
should create the concrete `agent_schedules` row bound to an org, agent,
channel, and any required connection.

Each schedule folder contains:

- `schedule.json`: install metadata, required integration, default cadence,
  resource selection hints, and instruction file path.
- `instructions.md`: the task prompt used each time the schedule runs.

These templates should be practical with a connected integration. Avoid
templates that require users to paste arbitrary URLs or manually wire external
systems outside the normal connection flow.
