"use client"

import { Button, Tabs } from "@heroui/react"
import { motion, MotionConfig } from "motion/react"
import { AppIcon } from "@/components/icon"
import { IntegrationLogo } from "@/components/integration-logo"

const catalogGroups = [
  {
    id: "operations",
    label: "Operations",
    agents: [
      {
        name: "Workflow coordinator",
        icon: "workflow",
        description:
          "Turns requests into assigned work, checks their status, and prepares the next handoff.",
        requirement: "Linear",
      },
      {
        name: "Vendor due diligence",
        icon: "file-text",
        description:
          "Reads security and purchasing documents, flags unanswered requirements, and drafts a review.",
        requirement: "Drive",
      },
    ],
  },
  {
    id: "support",
    label: "Support",
    agents: [
      {
        name: "Support investigator",
        icon: "headset",
        description:
          "Connects a customer report to prior tickets, product notes, and recent code changes.",
        requirement: "HubSpot",
      },
      {
        name: "Feedback analyst",
        icon: "messages-square",
        description:
          "Groups repeated customer requests and links each theme to the original conversations.",
        requirement: "Slack",
      },
    ],
  },
  {
    id: "product",
    label: "Product",
    agents: [
      {
        name: "Product researcher",
        icon: "search",
        description:
          "Pulls evidence for a product question and returns a short brief with cited sources.",
        requirement: "Notion",
      },
      {
        name: "Release checker",
        icon: "calendar",
        description:
          "Checks open issues, dependencies, and release notes before the team ships.",
        requirement: "GitHub",
      },
    ],
  },
  {
    id: "engineering",
    label: "Engineering",
    agents: [
      {
        name: "Bug investigator",
        icon: "code-xml",
        description:
          "Reproduces a reported failure, finds the first broken step, and proposes a focused fix.",
        requirement: "GitHub",
      },
      {
        name: "Incident reporter",
        icon: "radar",
        description:
          "Reads alerts and deploy history, then keeps the team updated in Slack.",
        requirement: "Slack",
      },
    ],
  },
] as const

const catalogBrandProviders: Record<string, string> = {
  GitHub: "github",
  Linear: "linear",
  Notion: "notion",
  Slack: "slack",
}

export function AgentCatalogExplorer() {
  return (
    <MotionConfig reducedMotion="user">
      <div className="overflow-hidden rounded-sm border border-border bg-surface shadow-xs">
        <div className="flex flex-col gap-4 border-b border-border px-5 py-5 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <p className="text-sm font-semibold">Choose an agent</p>
            <p className="mt-1 text-xs text-muted">
              Install a specialist for one team, then edit its setup.
            </p>
          </div>
          <div className="flex h-9 w-full max-w-64 items-center gap-2 rounded-sm border border-border bg-background px-3 text-xs text-muted">
            <AppIcon icon="search" size={14} />
            Search by job
          </div>
        </div>

        <Tabs
          variant="primary"
          defaultSelectedKey="operations"
          className="w-full p-4 sm:p-5"
        >
          <Tabs.ListContainer className="max-w-full overflow-x-auto">
            <Tabs.List
              aria-label="Browse agents by team"
              className="min-w-[570px]"
            >
              {catalogGroups.map((group) => (
                <Tabs.Tab id={group.id} key={group.id}>
                  {group.label}
                  <Tabs.Indicator />
                </Tabs.Tab>
              ))}
            </Tabs.List>
          </Tabs.ListContainer>

          {catalogGroups.map((group) => (
            <Tabs.Panel key={group.id} id={group.id} className="mt-5 p-0">
              <motion.div
                initial={{ opacity: 0, y: 6 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ duration: 0.3, ease: [0.16, 1, 0.3, 1] }}
                className="divide-y divide-border"
              >
                {group.agents.map((agent) => (
                  <div
                    key={agent.name}
                    className="grid gap-4 py-5 sm:grid-cols-[auto_1fr_auto] sm:items-center"
                  >
                    <span className="flex size-10 items-center justify-center rounded-sm bg-surface-secondary text-foreground">
                      <AppIcon icon={agent.icon} size={19} />
                    </span>
                    <div>
                      <p className="text-sm font-medium">{agent.name}</p>
                      <p className="mt-1 max-w-[62ch] text-xs leading-5 text-muted">
                        {agent.description}
                      </p>
                      <div className="mt-2 inline-flex items-center gap-1.5 text-[0.68rem] text-muted">
                        {catalogBrandProviders[agent.requirement] ? (
                          <IntegrationLogo
                            provider={catalogBrandProviders[agent.requirement]}
                            size={16}
                          />
                        ) : (
                          <AppIcon icon="plug" size={12} />
                        )}
                        Connects to {agent.requirement}
                      </div>
                    </div>
                    <Button size="sm" variant="secondary" className="w-fit">
                      Add to team
                    </Button>
                  </div>
                ))}
              </motion.div>
            </Tabs.Panel>
          ))}
        </Tabs>
      </div>
    </MotionConfig>
  )
}
