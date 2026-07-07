# GitHub webhook fixtures

All payloads sourced from https://github.com/octokit/webhooks/tree/main/payload-examples/api.github.com — the canonical fixture set GitHub itself ships, used by every official GitHub library.

| File | Source | Notes |
|---|---|---|
| `issues.opened.json` | `issues/opened.payload.json` | Codertocat opening issue #1 |
| `issues.labeled.json` | `issues/labeled.payload.json` | Issue gets a label |
| `pull_request.opened.json` | `pull_request/opened.payload.json` | PR #2, draft=false |
| `issue_comment.created.issue.json` | `issue_comment/created.payload.json` | Comment body patched to `@hivy can you triage this issue please?` for mention tests |
| `issue_comment.created.pr.json` | derived from `created.payload.json` | `issue.pull_request` field added (real shape: url/html_url/diff_url/patch_url); body patched to `@hivy can you take a look at this PR?`. GitHub uses this same shape when a comment is on a PR. |
| `push.json` | `push/payload.json` | Tag push (`refs/tags/simple-tag`) |
| `push.new-branch.json` | `push/with-new-branch.payload.json` | Branch push to `refs/heads/master` |
| `workflow_run.completed.json` | `workflow_run/completed.payload.json` | conclusion=success |
| `workflow_run.completed.failure.json` | derived | conclusion field patched to `failure` |
| `release.published.json` | `release/published.payload.json` | Release `0.0.1`, id=17372790 |
| `check_suite.completed.json` | `check_suite/completed.payload.json` | conclusion=success, suite id=118578147, one PR (#2) in `pull_requests[]` |
| `pull_request_review.submitted.json` | `pull_request_review/submitted.payload.json` | state=commented, review id=237895671, PR #2 |
| `pull_request_review_comment.created.json` | `pull_request_review_comment/created.payload.json` | comment id=284312630 on README.md line 265, PR #2 |
