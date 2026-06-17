"use client"

import { Button, Spinner } from "@heroui/react"
import type { components } from "@/lib/api/schema"
import type { InstalledAgent } from "../_lib"

export type AgentSandboxUpgrade =
  components["schemas"]["agentSandboxUpgradeResponse"]

export function SandboxRuntimeSection({
  agent,
  isBusy,
  onUpgrade,
}: {
  agent?: InstalledAgent
  upgrade?: AgentSandboxUpgrade
  isBusy: boolean
  onUpgrade: () => void
}) {
  const sandbox = agent?.sandbox
  const currentVersion = sandbox?.runtime_version?.trim() || "Unknown"
  const latestVersion = agent?.latest_runtime_version?.trim() || "Unknown"
  const upgradeAvailable = agent?.upgrade_available === true
  const showUpgradeButton = upgradeAvailable || isBusy
  const statusText = upgradeAvailable
    ? "There's a new version available"
    : "No new versions available. this is the latest version"

  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-medium">Your agent&apos;s computer</h2>
        <p className="text-sm text-muted">
          Keep your agent&apos;s computer up to date with the latest hivy
          runtime version
        </p>
      </div>

      <div className="flex items-center gap-4 rounded-2xl border border-border bg-surface px-4 py-4">
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="text-sm">
            Your agent&apos;s computer is currently at version {currentVersion}
          </span>
          <span className="text-sm text-muted">{statusText}</span>
        </div>
        {showUpgradeButton ? (
          <Button
            size="sm"
            variant="tertiary"
            isDisabled={isBusy || !upgradeAvailable}
            onPress={onUpgrade}
          >
            {isBusy ? <Spinner color="current" size="sm" /> : null}
            {isBusy ? "Upgrading…" : `Upgrade to ${latestVersion}`}
          </Button>
        ) : null}
      </div>
    </section>
  )
}
