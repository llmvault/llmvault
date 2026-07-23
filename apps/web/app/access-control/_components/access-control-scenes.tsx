"use client"

import { useState, type ReactNode } from "react"
import { Switch, Tabs } from "@heroui/react"
import { motion, MotionConfig } from "motion/react"
import { AppIcon } from "@/components/icon"
import { IntegrationLogo } from "@/components/integration-logo"

const easeOut = [0.16, 1, 0.3, 1] as const
const providerIcons = new Set(["chrome", "github", "linear", "notion", "slack"])

function ResourceIcon({ icon }: { icon: string }) {
  if (providerIcons.has(icon)) {
    return <IntegrationLogo provider={icon} size={32} className="rounded-sm" />
  }
  return (
    <span className="flex size-8 shrink-0 items-center justify-center rounded-sm bg-surface-secondary">
      <AppIcon icon={icon} size={15} />
    </span>
  )
}

function SceneFrame({
  label,
  children,
  className = "",
}: {
  label: string
  children: ReactNode
  className?: string
}) {
  return (
    <MotionConfig reducedMotion="user">
      <motion.div
        role="img"
        aria-label={label}
        initial={{ opacity: 0, y: 10 }}
        whileInView={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.48, ease: easeOut }}
        viewport={{ once: true, amount: 0.2 }}
        className={`overflow-hidden rounded-sm border border-border bg-surface shadow-xs ${className}`}
      >
        {children}
      </motion.div>
    </MotionConfig>
  )
}

const provisioningGroups = [
  {
    id: "connections",
    label: "Connections",
    icon: "plug",
    description: "Connected company accounts the team’s agents can use.",
    rows: [
      ["slack", "Slack · Atlas", "Approved channels and threads", true],
      ["linear", "Linear · Product", "Issues, projects, and comments", true],
      ["notion", "Notion · Product", "Team pages and databases", true],
      [
        "github",
        "GitHub · atlas-web",
        "Repositories, issues, and pull requests",
        true,
      ],
    ],
  },
  {
    id: "knowledge",
    label: "Knowledge sources",
    icon: "database",
    description: "Indexed company material the team’s agents can search.",
    rows: [
      ["notion", "Product handbook", "Approved Notion pages", true],
      ["slack", "Customer conversations", "Selected Slack channels", true],
      ["github", "Engineering decisions", "Repository documents", false],
      ["chrome", "Public help center", "Approved website pages", true],
    ],
  },
  {
    id: "skills",
    label: "Skills",
    icon: "sparkles",
    description: "Published instructions the team’s agents can run.",
    rows: [
      ["file-text", "Write a decision record", "Product operations", true],
      ["message-square", "Sort customer feedback", "Customer research", true],
      ["git-branch", "Review a pull request", "Engineering", false],
      ["presentation", "Draft a launch brief", "Go-to-market", true],
    ],
  },
] as const

type ProvisioningGroup = (typeof provisioningGroups)[number]

function ProvisioningRows({ group }: { group: ProvisioningGroup }) {
  const [selected, setSelected] = useState<Set<string>>(
    () => new Set(group.rows.filter((row) => row[3]).map((row) => row[1]))
  )

  function toggle(name: string, on: boolean) {
    setSelected((current) => {
      const next = new Set(current)
      if (on) next.add(name)
      else next.delete(name)
      return next
    })
  }

  return (
    <motion.div
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.32, ease: easeOut }}
    >
      <div className="flex items-end justify-between gap-5 border-b border-border pb-4">
        <div>
          <p className="text-sm font-medium">Product team access</p>
          <p className="mt-1 text-xs text-muted">{group.description}</p>
        </div>
        <span className="shrink-0 text-[0.68rem] text-muted">
          {selected.size} available
        </span>
      </div>
      <div className="divide-y divide-border">
        {group.rows.map(([icon, name, note]) => {
          const on = selected.has(name)
          return (
            <div key={name} className="flex items-center gap-3 py-3.5">
              <ResourceIcon icon={icon} />
              <div className="min-w-0 flex-1">
                <p className="truncate text-xs font-medium">{name}</p>
                <p className="mt-0.5 truncate text-[0.68rem] text-muted">
                  {note}
                </p>
              </div>
              <Switch
                aria-label={`${on ? "Remove" : "Allow"} ${name} for Product team`}
                isSelected={on}
                onChange={(next) => toggle(name, next)}
                className="shrink-0"
              >
                <Switch.Control>
                  <Switch.Thumb />
                </Switch.Control>
              </Switch>
            </div>
          )
        })}
      </div>
    </motion.div>
  )
}

export function ProvisioningPreview() {
  return (
    <SceneFrame label="Connections, knowledge sources, and skills granted to the Product team">
      <div className="p-5 md:p-7">
        <Tabs
          variant="primary"
          defaultSelectedKey="connections"
          className="w-full"
        >
          <Tabs.ListContainer className="max-w-full overflow-x-auto">
            <Tabs.List
              aria-label="Choose resources for the Product team"
              className="w-fit min-w-[520px]"
            >
              {provisioningGroups.map((group) => (
                <Tabs.Tab id={group.id} key={group.id}>
                  <span className="flex items-center gap-2 whitespace-nowrap">
                    <AppIcon icon={group.icon} size={14} />
                    {group.label}
                  </span>
                  <Tabs.Indicator />
                </Tabs.Tab>
              ))}
            </Tabs.List>
          </Tabs.ListContainer>

          {provisioningGroups.map((group) => (
            <Tabs.Panel id={group.id} key={group.id} className="mt-7 p-0">
              <ProvisioningRows group={group} />
            </Tabs.Panel>
          ))}
        </Tabs>
      </div>
    </SceneFrame>
  )
}
