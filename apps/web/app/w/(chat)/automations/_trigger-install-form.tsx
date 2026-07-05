"use client"

import {
  automationTriggerKey,
  type AutomationItem,
  type InstalledTrigger,
} from "@/app/w/(chat)/automations/_data"
import { SlackReactionInstallForm } from "@/app/w/(chat)/automations/_trigger-install-form-slack"
import { GithubMentionInstallForm } from "@/app/w/(chat)/automations/_trigger-install-form-github"
import {
  isGithubMentionKey,
  slackReactionKey,
} from "@/app/w/(chat)/automations/_trigger-install-form-shared"

export function TriggerInstallForm({
  automation,
  trigger,
}: {
  automation: AutomationItem
  trigger?: InstalledTrigger
}) {
  const triggerKey = trigger?.trigger_key || automationTriggerKey(automation)

  if (automation.provider === "slack" && triggerKey === slackReactionKey) {
    return (
      <SlackReactionInstallForm automation={automation} trigger={trigger} />
    )
  }

  if (automation.provider === "github-app" && isGithubMentionKey(triggerKey)) {
    return <GithubMentionInstallForm automation={automation} trigger={trigger} />
  }

  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-semibold text-foreground">
          Trigger setup unavailable
        </h2>
        <p className="text-muted-foreground mt-1 text-sm leading-5">
          This trigger template is not supported by the installer yet.
        </p>
      </div>
    </section>
  )
}
