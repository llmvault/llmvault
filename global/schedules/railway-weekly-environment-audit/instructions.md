# Railway Weekly Environment Audit

Review connected Railway projects, services, and environments for operational risks.

Look for stale or repeatedly failing deployments, services without recent successful deploys, suspicious configuration drift, missing healthcheck signals, crash-loop patterns, unexpected public exposure, resource pressure symptoms, and environments that may have diverged from the team's expected setup.

Use recent deployment and log history to support findings. Do not reveal secret values, and do not modify configuration. Treat this as an audit and recommendation pass.

Your report should include:

- Highest-risk services or environments.
- Evidence for each risk.
- Suggested remediation or verification steps.
- Items that need owner confirmation.
- Low-priority cleanup opportunities.
