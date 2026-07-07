"use client"

import {
  type AutomationItem,
  type InstalledTrigger,
} from "@/app/w/(chat)/automations/_data"
import {
  GithubMentionInstallFormBase,
  type GithubMentionFormConfig,
} from "@/app/w/(chat)/automations/_trigger-install-form-github-base"

// The code-reviews app can also auto-review every new pull request. This form
// binds to github-app-code-reviews connections and installs the pr_opened
// trigger key; it reuses the shared GitHub repo form body with review-oriented
// copy that fires when any pull request opens in the selected repo — no mention.
const githubCodeReviewsPrOpenedConfig: GithubMentionFormConfig = {
  provider: "github-app-code-reviews",
  toastNoun: "GitHub auto-review trigger",
  repoDescription:
    "Choose the repository where every new pull request should be reviewed automatically.",
  agentDescription:
    "Select the agent that should review pull requests opened in this repository.",
  instructionsDescription:
    "These instructions are added to the agent run when a pull request is opened in this repository.",
  existingWarning:
    "This agent already has an auto-review trigger for this repository.",
  channelDescription:
    "The channel this agent's review session runs in. You can only pick channels you have access to.",
  statusEnabledDescription:
    "New pull requests in this repository will be reviewed automatically.",
  statusDisabledDescription:
    "New pull requests in this repository will not be reviewed.",
}

export function GithubCodeReviewsPrOpenedInstallForm({
  automation,
  trigger,
}: {
  automation: AutomationItem
  trigger?: InstalledTrigger
}) {
  return (
    <GithubMentionInstallFormBase
      automation={automation}
      trigger={trigger}
      config={githubCodeReviewsPrOpenedConfig}
    />
  )
}
