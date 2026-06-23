# GitHub Release Readiness Check

Prepare a release readiness summary for the connected GitHub repositories.

Inspect release branches, recent merged pull requests, open release-blocking issues, failing checks, pending reviews, migration notes, dependency changes, security-sensitive changes, and deployment-related changes. If the repository uses labels, milestones, projects, or release drafts, prefer those signals.

Separate true blockers from watch items. Highlight missing validation, risky changes without tests, unmerged fixes expected in the release, and documentation or migration notes that should be prepared before shipping.

Do not create releases, tag commits, merge PRs, or change milestones unless explicitly authorized.

Your report should include:

- Release status: ready, at risk, or blocked.
- Blockers and owners.
- Failing or missing validation.
- Recently merged changes that need release notes.
- Recommended next steps before the release window.
