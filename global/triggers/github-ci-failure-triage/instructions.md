# GitHub CI Failure Triage

You are responding to a completed GitHub Actions workflow run that did not pass.

Follow these steps in order:

1. Load the `git-github` skill first because this task works with GitHub workflow and pull request context.
2. Identify the workflow run context: repository, workflow name, run ID or URL, branch, commit SHA, actor, conclusion, and whether the run is associated with a pull request.
3. Fetch the workflow run, jobs, failed steps, annotations, and available logs through the GitHub integration.
4. Find the primary failure. Prefer the first failing job or step and any explicit error annotation over later cascading failures.
5. If logs are unavailable, state that clearly and continue using only the run metadata, job names, step names, and annotations.
6. Classify the likely cause as one of: code change, flaky test, infrastructure/runtime issue, missing secrets or permissions, dependency installation, lint/typecheck failure, timeout, or unknown.
7. If the run is associated with a pull request, inspect the PR title, diff, and recent commits before recommending a fix.
8. Use evidence from logs, annotations, changed files, and commit context before assigning blame. Avoid speculation when the evidence is weak.
9. Choose the smallest concrete next action. For flaky or infrastructure-looking failures, recommend a safe verification step such as rerunning failed jobs, but do not rerun jobs, push commits, or change workflow configuration unless the user explicitly authorized that behavior.
10. Prepare a concise triage summary with:
   - The failing workflow, job, and step.
   - The most likely root cause with decisive log evidence.
   - Whether the failure looks code-caused, flaky, or infrastructure-related.
   - The smallest concrete next action.
   - A proposed fix or investigation command when useful.
11. If the failed workflow run is associated with a pull request, post the triage summary as a comment on that pull request. Put the entire comment body under a markdown accordion so the PR stays clean:
   ```markdown
   <details>
   <summary>CI failure triage</summary>

   [triage summary goes here]

   </details>
   ```
12. If the failed workflow run is not associated with a pull request, do not post a GitHub comment. Respond directly with the triage summary instead.

Keep the summary concise. Do not paste long logs; quote only the decisive lines.
