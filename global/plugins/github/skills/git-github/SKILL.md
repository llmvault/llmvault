---
name: git-github
description: Use for Git and GitHub work: repository conventions, branches, commits, pull requests, comments, labels, CI status, reviews, releases, and Drive-hosted media.
---

# Git and GitHub through Hivy

Hivy supplies short-lived GitHub App authentication for `gh` and git. Never run `gh auth login`, `gh auth status`, or browser authentication; an auth failure requires a GitHub connection or enabled plugin, not a terminal workaround.

GitHub comments, reviews, and check events are delivered to this session. Act on the event and end the turn; do not poll for later activity unless the user explicitly asks.

## Before changing a repository

Read `AGENTS.md`, contribution guidance, the PR template, existing tests, and recent commits/branches. Match the repository's branch, commit, PR, and label conventions rather than inventing one. Stage intended paths and inspect the diff before committing; never bypass a failed hook without user approval or force-push a shared/default branch.

## Normal delivery flow

1. Start from the current default branch and create a focused branch following observed convention.
2. Make and verify the requested change.
3. Commit and push the intended files.
4. Read the PR template and create the PR non-interactively with its required sections, summary, and verification evidence.
5. Respond to delivered feedback on GitHub, then end the turn.

```bash
git fetch origin
BASE=$(gh repo view --json defaultBranchRef -q .defaultBranchRef.name)
git switch "$BASE" && git pull --ff-only
git switch -c <branch>
git status && git diff
git add <intended-paths>
git commit -m "<message matching repo convention>"
git push -u origin HEAD
gh pr create --base "$BASE" --title "<title>" --body-file pr.md
```

Use `gh pr edit`, `gh pr comment`, `gh issue comment`, `gh label list`, and `gh pr checks` only for the work requested or an event just delivered. Do not create labels unless asked. Never merge another author's PR without explicit user approval.

## Review and media rules

For inline code-review comments, use the exact review payload and diff-anchor rules in the GitHub Code Reviews skill; generic `gh` knowledge is not enough to safely reconstruct them.

For screenshots, demos, or other PR media, load `drive`, upload the asset through its documented flow, and embed the returned URL. Do not commit assets only to host them, use GitHub attachments/gists, data URIs, or a third-party host.

## Handoff

Report the branch, commit, PR URL, verification, and any remaining blocker. GitHub itself is the place for PR discussion; keep the channel summary short.
