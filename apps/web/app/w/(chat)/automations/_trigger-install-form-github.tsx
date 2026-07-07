"use client"

import {
  type AutomationItem,
  type InstalledTrigger,
} from "@/app/w/(chat)/automations/_data"
import {
  GithubMentionInstallFormBase,
  type GithubMentionFormConfig,
} from "@/app/w/(chat)/automations/_trigger-install-form-github-base"

const githubMentionConfig: GithubMentionFormConfig = {
  provider: "github-app",
  toastNoun: "GitHub mention trigger",
  repoDescription:
    "Choose the repository where @mentions should trigger the agent.",
  agentDescription:
    "Select the agent that should handle @mentions in this repository.",
  instructionsDescription:
    "These instructions are added to the agent run when someone @mentions Hivy on an issue or pull request.",
  existingWarning:
    "This agent already has a mention trigger for this repository.",
}

export function GithubMentionInstallForm({
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
      config={githubMentionConfig}
    />
  )
}
