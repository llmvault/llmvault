# GitHub Daily CI Failure Digest

Review GitHub Actions workflow runs from the last 24 hours and summarize failures that still matter.

Fetch failed, timed out, cancelled, or action-required workflow runs. For each significant failure, inspect the workflow, branch, commit, pull request if present, failed jobs, annotations, and available logs. Group repeated failures by workflow, job, failing step, error signature, and affected repository.

Distinguish code regressions from flaky tests, missing secrets or permissions, dependency issues, runner capacity, timeouts, and infrastructure failures. Prefer a small number of high-confidence findings over a long list of every failed run.

Do not rerun jobs, change workflow files, or push fixes unless explicitly authorized.

Your digest should include:

- New or recurring CI failure groups.
- The most likely cause for each group.
- Affected repositories, branches, or PRs.
- Suggested owner or follow-up.
- Failures that look flaky and should be monitored or quarantined.
