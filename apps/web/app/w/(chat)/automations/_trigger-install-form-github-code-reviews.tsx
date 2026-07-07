"use client"

import {
  type AutomationItem,
  type InstalledTrigger,
} from "@/app/w/(chat)/automations/_data"
import {
  GithubMentionInstallFormBase,
  type GithubMentionFormConfig,
} from "@/app/w/(chat)/automations/_trigger-install-form-github-base"

// The code-reviews app answers @usehivy-reviews on pull requests only. It binds
// to github-app-code-reviews connections and reuses the pr_mention trigger key;
// the connection provider is what differentiates it from the primary GitHub App
// mention trigger.
const githubCodeReviewsConfig: GithubMentionFormConfig = {
  provider: "github-app-code-reviews",
  toastNoun: "GitHub code review trigger",
  repoDescription:
    "Choose the repository where @usehivy-reviews review requests should trigger the agent.",
  agentDescription:
    "Select the agent that should review pull requests when @usehivy-reviews is @mentioned in this repository.",
  instructionsDescription:
    "These instructions are added to the agent run when someone @mentions @usehivy-reviews on a pull request.",
  existingWarning:
    "This agent already has a code review trigger for this repository.",
}

export function GithubCodeReviewsInstallForm({
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
      config={githubCodeReviewsConfig}
    />
  )
}
