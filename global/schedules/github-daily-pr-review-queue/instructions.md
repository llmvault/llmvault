# GitHub Daily PR Review Queue

Review open pull requests for the connected GitHub repositories and prepare a concise review queue update.

Focus on PRs that need human attention: requested reviews, stale review threads, unresolved requested changes, failing checks, merge conflicts, large diffs, missing tests, blocked dependencies, and PRs that have not moved recently. Prefer repository-specific conventions for labels, reviewers, and branch names when available.

For each important PR, report the title, author, age, review state, CI state, risk level, and the next best action. Group low-risk or inactive PRs separately so the team can scan quickly.

Do not approve, request changes, merge, push commits, or close PRs. If you recommend an action that changes repository state, present it as a suggested next step.

Your update should include:

- The highest-priority PRs to review today.
- PRs blocked by failing checks or requested changes.
- Stale PRs that need owner follow-up.
- Large or risky PRs that deserve careful review.
- A short recommended review order.
