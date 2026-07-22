"use client"

import { useState, type ReactNode } from "react"
import { Button, Chip, Switch, Tabs } from "@heroui/react"
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

const reveal = {
  hidden: { opacity: 0, y: 10 },
  show: (delay = 0) => ({
    opacity: 1,
    y: 0,
    transition: { duration: 0.48, delay, ease: easeOut },
  }),
}

function SceneFrame({ label, children, className = "" }: { label: string; children: ReactNode; className?: string }) {
  return (
    <MotionConfig reducedMotion="user">
      <motion.div
        role="img"
        aria-label={label}
        initial="hidden"
        whileInView="show"
        viewport={{ once: true, amount: 0.2 }}
        className={`overflow-hidden rounded-sm border border-border bg-surface shadow-xs ${className}`}
      >
        {children}
      </motion.div>
    </MotionConfig>
  )
}

function Initials({ children }: { children: ReactNode }) {
  return <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-default text-[0.68rem] font-medium text-muted">{children}</span>
}

const roles = [
  {
    id: "owner",
    label: "Owner",
    title: "One accountable owner",
    description: "The owner can do everything an admin can, then handles the workspace actions that should stay with one person.",
    permissions: [
      ["Manage members, teams, and shared resources", true],
      ["Transfer workspace ownership", true],
      ["Delete the workspace", true],
      ["Use agents in joined teams", true],
    ],
  },
  {
    id: "admin",
    label: "Admin",
    title: "Workspace setup without ownership",
    description: "Admins organize people and teams, connect company tools, and decide which shared resources each team receives.",
    permissions: [
      ["Manage members, teams, and invitations", true],
      ["Grant connections, skills, and knowledge", true],
      ["Transfer workspace ownership", false],
      ["Use agents across managed teams", true],
    ],
  },
  {
    id: "member",
    label: "Member",
    title: "Access for the work at hand",
    description: "Members work with agents and resources inside their active teams without changing workspace-wide membership or provisioning.",
    permissions: [
      ["Use and manage own-team agents", true],
      ["Use resources granted to the team", true],
      ["Change workspace membership", false],
      ["Grant shared resources to a team", false],
    ],
  },
] as const

function RoleAccessPreview() {
  return (
    <SceneFrame label="Workspace role comparison for owner, admin, and member">
      <div className="border-b border-border px-5 py-4">
        <p className="text-sm font-medium">Workspace roles</p>
        <p className="mt-1 text-xs text-muted">Choose the smallest role that matches the job.</p>
      </div>
      <div className="p-5 md:p-7">
        <Tabs variant="primary" defaultSelectedKey="admin" className="w-full">
          <Tabs.ListContainer className="max-w-full overflow-x-auto">
            <Tabs.List aria-label="Compare Hivy workspace roles" className="w-fit">
              {roles.map((role) => (
                <Tabs.Tab id={role.id} key={role.id}>
                  {role.label}
                  <Tabs.Indicator />
                </Tabs.Tab>
              ))}
            </Tabs.List>
          </Tabs.ListContainer>

          {roles.map((role) => (
            <Tabs.Panel id={role.id} key={role.id} className="mt-7 p-0">
              <motion.div
                initial={{ opacity: 0, y: 6 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ duration: 0.32, ease: easeOut }}
                className="grid gap-7 lg:grid-cols-[0.76fr_1.24fr]"
              >
                <div className="max-w-[36ch]">
                  <p className="text-lg font-medium tracking-[-0.025em]">{role.title}</p>
                  <p className="mt-3 text-sm leading-6 text-muted">{role.description}</p>
                </div>
                <div className="divide-y divide-border border-y border-border">
                  {role.permissions.map(([permission, allowed]) => (
                    <div key={permission} className="flex items-center gap-3 py-3.5">
                      <span
                        className={`flex size-5 shrink-0 items-center justify-center rounded-full ${allowed ? "bg-accent-soft text-foreground" : "bg-default text-muted"}`}
                      >
                        <AppIcon icon={allowed ? "check" : "x"} size={11} />
                      </span>
                      <span className={`text-xs ${allowed ? "text-foreground" : "text-muted"}`}>{permission}</span>
                    </div>
                  ))}
                </div>
              </motion.div>
            </Tabs.Panel>
          ))}
        </Tabs>
      </div>
    </SceneFrame>
  )
}

const members = [
  ["MC", "Maya Chen", "maya@northstar.example", "Admin"],
  ["LB", "Leah Brooks", "leah@northstar.example", "Member"],
  ["OA", "Omar Ali", "omar@northstar.example", "Member"],
  ["JN", "Jon Nolan", "jon@northstar.example", "Member"],
] as const

function MembershipPreview() {
  return (
    <SceneFrame label="Workspace members assigned to the Product team">
      <div className="grid lg:grid-cols-[0.9fr_1.1fr]">
        <motion.div variants={reveal} custom={0.05} className="border-b border-border p-5 lg:border-r lg:border-b-0 lg:p-7">
          <div className="flex items-center justify-between gap-4">
            <div>
              <p className="text-sm font-medium">Workspace members</p>
              <p className="mt-1 text-xs text-muted">4 active</p>
            </div>
            <Button size="sm" variant="tertiary">
              <AppIcon icon="user-plus" size={14} /> Invite
            </Button>
          </div>
          <div className="mt-5 divide-y divide-border border-y border-border">
            {members.map(([initials, name, email, role]) => (
              <div key={email} className="flex items-center gap-3 py-3">
                <Initials>{initials}</Initials>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-xs font-medium">{name}</p>
                  <p className="mt-0.5 truncate text-[0.68rem] text-muted">{email}</p>
                </div>
                <span className="text-[0.65rem] text-muted">{role}</span>
              </div>
            ))}
          </div>
        </motion.div>

        <motion.div variants={reveal} custom={0.14} className="bg-surface-secondary p-5 lg:p-7">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="text-sm font-medium">Product team</p>
              <p className="mt-1 text-xs text-muted">Members who can reach Product agents and resources.</p>
            </div>
            <Chip size="sm">3 members</Chip>
          </div>
          <div className="mt-6 space-y-2">
            {members.slice(0, 3).map(([initials, name, email]) => (
              <div key={email} className="flex items-center gap-3 rounded-sm bg-surface px-3 py-3 shadow-xs">
                <Initials>{initials}</Initials>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-xs font-medium">{name}</p>
                  <p className="mt-0.5 text-[0.68rem] text-muted">Team member</p>
                </div>
                <Button size="sm" variant="ghost">
                  Remove
                </Button>
              </div>
            ))}
          </div>
          <div className="mt-5 flex items-center gap-2 text-[0.68rem] text-muted">
            <AppIcon icon="info" size={13} />
            Jon remains in the workspace, but not in Product.
          </div>
        </motion.div>
      </div>
    </SceneFrame>
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
      ["github", "GitHub · atlas-web", "Repositories, issues, and pull requests", true],
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
  const [selected, setSelected] = useState<Set<string>>(() => new Set(group.rows.filter((row) => row[3]).map((row) => row[1])))

  function toggle(name: string, on: boolean) {
    setSelected((current) => {
      const next = new Set(current)
      if (on) next.add(name)
      else next.delete(name)
      return next
    })
  }

  return (
    <motion.div initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.32, ease: easeOut }}>
      <div className="flex items-end justify-between gap-5 border-b border-border pb-4">
        <div>
          <p className="text-sm font-medium">Product team access</p>
          <p className="mt-1 text-xs text-muted">{group.description}</p>
        </div>
        <span className="shrink-0 text-[0.68rem] text-muted">{selected.size} available</span>
      </div>
      <div className="divide-y divide-border">
        {group.rows.map(([icon, name, note]) => {
          const on = selected.has(name)
          return (
            <div key={name} className="flex items-center gap-3 py-3.5">
              <ResourceIcon icon={icon} />
              <div className="min-w-0 flex-1">
                <p className="truncate text-xs font-medium">{name}</p>
                <p className="mt-0.5 truncate text-[0.68rem] text-muted">{note}</p>
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
        <Tabs variant="primary" defaultSelectedKey="connections" className="w-full">
          <Tabs.ListContainer className="max-w-full overflow-x-auto">
            <Tabs.List aria-label="Choose resources for the Product team" className="w-fit min-w-[520px]">
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

function AgentBoundaryPreview() {
  return (
    <SceneFrame label="Research agent inheriting access from the Product team">
      <div className="flex items-center justify-between border-b border-border px-5 py-4">
        <div className="flex items-center gap-3">
          <span className="flex size-8 items-center justify-center rounded-sm bg-accent-soft">
            <AppIcon icon="bot" size={16} />
          </span>
          <div>
            <p className="text-sm font-medium">Research agent</p>
            <p className="text-[0.68rem] text-muted">Product team</p>
          </div>
        </div>
        <Chip size="sm" color="accent">
          Active
        </Chip>
      </div>
      <div className="grid lg:grid-cols-[0.88fr_1.12fr]">
        <motion.div variants={reveal} custom={0.08} className="border-b border-border p-5 lg:border-r lg:border-b-0 lg:p-7">
          <p className="text-xs font-medium">Inherited from Product</p>
          <div className="mt-5 space-y-4">
            {[
              ["linear", "Linear · Product", "Connection"],
              ["notion", "Product handbook", "Knowledge source"],
              ["sparkles", "Write a decision note", "Skill"],
              ["slack", "Slack · Northstar", "Connection"],
            ].map(([icon, name, kind]) => (
              <div key={name} className="flex items-center gap-3">
                <ResourceIcon icon={icon} />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-xs font-medium">{name}</p>
                  <p className="mt-0.5 text-[0.68rem] text-muted">{kind}</p>
                </div>
                <AppIcon icon="check-circle" size={14} className="text-success" />
              </div>
            ))}
          </div>
        </motion.div>

        <motion.div variants={reveal} custom={0.16} className="bg-surface-secondary p-5 lg:p-7">
          <div>
            <p className="text-xs font-medium">Agent connection access</p>
            <p className="mt-1 text-[0.68rem] leading-5 text-muted">
              Connections are inherited from the team. Turn one off for this agent without changing the team grant.
            </p>
          </div>
          <div className="mt-5 overflow-hidden rounded-sm border border-border bg-surface">
            {[
              ["linear", "Linear · Product", "Enabled"],
              ["notion", "Notion · Product", "Enabled"],
              ["slack", "Slack · Northstar", "Enabled"],
              ["github", "GitHub · northstar-app", "Off for agent"],
            ].map(([provider, label, on], index) => (
              <div key={String(label)} className={`flex items-center justify-between gap-4 px-4 py-3 ${index < 3 ? "border-b border-border" : ""}`}>
                <span className="flex min-w-0 items-center gap-2 text-xs">
                  <IntegrationLogo provider={provider} size={24} />
                  <span className="truncate">{label}</span>
                </span>
                <span className={`text-[0.65rem] font-medium ${on === "Enabled" ? "text-foreground" : "text-muted"}`}>{on}</span>
              </div>
            ))}
          </div>
        </motion.div>
      </div>
    </SceneFrame>
  )
}

function ExternalRoutePreview() {
  return (
    <SceneFrame label="External Slack channel routed to one agent on the Support team">
      <div className="border-b border-border px-5 py-4">
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-sm font-medium">External routing</p>
            <p className="mt-1 text-xs text-muted">Support team</p>
          </div>
          <Button size="sm">Add route</Button>
        </div>
      </div>
      <div className="bg-surface-secondary p-5 md:p-7">
        <motion.div variants={reveal} custom={0.08} className="grid items-center gap-4 lg:grid-cols-[1fr_auto_1fr_auto_1fr]">
          <div className="rounded-sm border border-border bg-surface p-4">
            <p className="text-[0.65rem] font-medium text-muted">Connection</p>
            <div className="mt-3 flex items-center gap-2 text-xs font-medium">
              <IntegrationLogo provider="slack" size={24} /> Slack · Northstar
            </div>
          </div>
          <AppIcon icon="arrow-right" size={16} className="mx-auto rotate-90 text-muted lg:rotate-0" />
          <div className="rounded-sm border border-border bg-surface p-4">
            <p className="text-[0.65rem] font-medium text-muted">Provider resource</p>
            <div className="mt-3 flex items-center gap-2 text-xs font-medium">
              <AppIcon icon="hash" size={16} /> support-escalations
            </div>
          </div>
          <AppIcon icon="arrow-right" size={16} className="mx-auto rotate-90 text-muted lg:rotate-0" />
          <div className="rounded-sm border border-border bg-surface p-4">
            <p className="text-[0.65rem] font-medium text-muted">Assigned agent</p>
            <div className="mt-3 flex items-center gap-2 text-xs font-medium">
              <AppIcon icon="headset" size={16} /> Support agent
            </div>
          </div>
        </motion.div>
        <motion.div variants={reveal} custom={0.16} className="mt-5 flex items-start gap-3 border-t border-border pt-5">
          <AppIcon icon="route" size={17} className="mt-0.5 text-muted" />
          <div>
            <p className="text-xs font-medium">New conversations follow this route.</p>
            <p className="mt-1 text-[0.68rem] leading-5 text-muted">If a conversation already belongs to an agent, Hivy keeps that existing assignment.</p>
          </div>
        </motion.div>
      </div>
    </SceneFrame>
  )
}

const decisions = [
  {
    id: "available",
    label: "Granted resource",
    title: "Available to the agent",
    result: "Product handbook",
    detail: "The source belongs to Northstar and is granted to Product.",
    checks: [
      ["Active workspace member", true],
      ["Member of Product", true],
      ["Agent owned by Product", true],
      ["Source granted to Product", true],
    ],
  },
  {
    id: "outside",
    label: "Outside the team",
    title: "Not available to the agent",
    result: "Support incident archive",
    detail: "The source exists in Northstar, but Support owns its grant.",
    checks: [
      ["Active workspace member", true],
      ["Member of Product", true],
      ["Agent owned by Product", true],
      ["Source granted to Product", false],
    ],
  },
] as const

function AccessDecisionPreview() {
  return (
    <SceneFrame label="Hivy access check for granted and outside-team resources">
      <div className="p-5 md:p-7">
        <Tabs variant="primary" defaultSelectedKey="outside" className="w-full">
          <Tabs.ListContainer className="max-w-full overflow-x-auto">
            <Tabs.List aria-label="Access decision examples" className="w-fit">
              {decisions.map((decision) => (
                <Tabs.Tab id={decision.id} key={decision.id}>
                  {decision.label}
                  <Tabs.Indicator />
                </Tabs.Tab>
              ))}
            </Tabs.List>
          </Tabs.ListContainer>
          {decisions.map((decision) => (
            <Tabs.Panel id={decision.id} key={decision.id} className="mt-7 p-0">
              <motion.div
                initial={{ opacity: 0, y: 6 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ duration: 0.32, ease: easeOut }}
                className="grid gap-7 lg:grid-cols-[1.1fr_0.9fr]"
              >
                <div className="divide-y divide-border border-y border-border">
                  {decision.checks.map(([label, passed]) => (
                    <div key={label} className="flex items-center justify-between gap-4 py-3.5">
                      <span className="text-xs text-muted">{label}</span>
                      <span className="flex items-center gap-2 text-xs font-medium">
                        <AppIcon icon={passed ? "check-circle" : "circle-slash"} size={14} className={passed ? "text-success" : "text-muted"} />
                        {passed ? "Pass" : "No grant"}
                      </span>
                    </div>
                  ))}
                </div>
                <div className={`flex flex-col justify-between rounded-sm p-5 ${decision.id === "available" ? "bg-accent-soft" : "bg-surface-secondary"}`}>
                  <AppIcon icon={decision.id === "available" ? "check-circle" : "eye-off"} size={22} />
                  <div className="mt-12">
                    <p className="text-lg font-medium tracking-[-0.025em]">{decision.title}</p>
                    <p className="mt-2 text-xs font-medium">{decision.result}</p>
                    <p className="mt-2 text-[0.68rem] leading-5 text-muted">{decision.detail}</p>
                  </div>
                </div>
              </motion.div>
            </Tabs.Panel>
          ))}
        </Tabs>
      </div>
    </SceneFrame>
  )
}
