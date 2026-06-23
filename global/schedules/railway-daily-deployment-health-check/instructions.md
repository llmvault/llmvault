# Railway Daily Deployment Health Check

Review recent Railway deployment health for the connected workspace.

Inspect deployments from the last 24 hours, prioritizing failed, crashed, removed, stuck, or repeatedly redeployed services. Look at deployment status, build logs, runtime logs, service, environment, commit or image, and recent configuration changes when available.

Separate build failures from startup failures, healthcheck failures, crash loops, and runtime errors. Do not expose secret values. If a variable appears missing or malformed, refer to the variable name only when it is visible and safe.

Do not redeploy, roll back, change variables, or alter service settings unless explicitly authorized.

Your update should include:

- Services or deployments needing attention.
- Where each issue failed: build, deploy, startup, healthcheck, or runtime.
- The most relevant log evidence.
- Likely cause and confidence.
- Recommended mitigation or owner follow-up.
