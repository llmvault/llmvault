"use client"

import type { ReactNode } from "react"
import { Tabs } from "@heroui/react"
import { motion, MotionConfig } from "motion/react"
import { AppIcon } from "@/components/icon"

const easeOut = [0.16, 1, 0.3, 1] as const

const files = [
  {
    name: "refund-label.jpg",
    path: "customer-uploads",
    owner: "Uploaded by Maya",
    type: "image",
    size: "2.4 MB",
    updated: "2 min ago",
    icon: "image",
  },
  {
    name: "escalation-call.m4a",
    path: "call-recordings/july",
    owner: "Uploaded by Omar",
    type: "audio",
    size: "8.7 MB",
    updated: "18 min ago",
    icon: "mic",
  },
  {
    name: "accounts-at-risk.csv",
    path: "renewal-reviews",
    owner: "Created by Revenue agent",
    type: "output",
    size: "84 KB",
    updated: "Today, 11:42",
    icon: "table",
  },
  {
    name: "checkout-incident.md",
    path: "incident-notes",
    owner: "Created by Support agent",
    type: "output",
    size: "12 KB",
    updated: "Yesterday",
    icon: "file-text",
  },
] as const

const filters = [
  { id: "all", label: "Everything" },
  { id: "image", label: "Images" },
  { id: "audio", label: "Audio" },
  { id: "output", label: "Agent-created" },
] as const

function PreviewWindow({ title, action, children, className = "" }: { title: string; action?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <div className={`overflow-hidden rounded-sm border border-border bg-surface shadow-xs ${className}`}>
      <div className="flex min-h-12 items-center justify-between gap-4 border-b border-border px-4 md:px-5">
        <div className="flex min-w-0 items-center gap-2.5">
          <span className="size-2 rounded-full bg-muted/35" />
          <p className="truncate text-sm font-medium">{title}</p>
        </div>
        {action}
      </div>
      {children}
    </div>
  )
}

function FileRows({ filter }: { filter: (typeof filters)[number]["id"] }) {
  const visibleFiles = filter === "all" ? files : files.filter((file) => file.type === filter)

  return (
    <div className="min-h-[286px]">
      <div className="hidden grid-cols-[minmax(0,1.5fr)_minmax(120px,0.8fr)_90px_110px] gap-4 border-b border-border px-5 py-3 text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase md:grid">
        <span>File</span>
        <span>Location</span>
        <span>Size</span>
        <span>Modified</span>
      </div>
      {visibleFiles.map((file, index) => (
        <motion.div
          key={file.name}
          initial={{ opacity: 0, y: 7 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: index * 0.045, duration: 0.32, ease: easeOut }}
          className="grid gap-3 border-b border-border px-4 py-4 last:border-b-0 md:grid-cols-[minmax(0,1.5fr)_minmax(120px,0.8fr)_90px_110px] md:items-center md:gap-4 md:px-5"
        >
          <div className="flex min-w-0 items-center gap-3">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-sm bg-surface-secondary text-muted">
              <AppIcon icon={file.icon} size={16} />
            </span>
            <div className="min-w-0">
              <p className="truncate text-sm font-medium">{file.name}</p>
              <p className="mt-0.5 truncate text-xs text-muted md:hidden">
                {file.path} · {file.size}
              </p>
              <p className="mt-0.5 truncate text-[0.7rem] text-muted">{file.owner}</p>
            </div>
          </div>
          <span className="hidden truncate text-xs text-muted md:block">{file.path}</span>
          <span className="hidden text-xs text-muted md:block">{file.size}</span>
          <span className="hidden text-xs text-muted md:block">{file.updated}</span>
        </motion.div>
      ))}
    </div>
  )
}

export function DriveBrowserPreview() {
  return (
    <MotionConfig reducedMotion="user">
      <PreviewWindow
        title="Support agent / Files"
        action={
          <span className="inline-flex items-center gap-1.5 text-xs text-muted">
            <AppIcon icon="upload" size={13} /> Add file
          </span>
        }
      >
        <div className="grid min-h-[470px] md:grid-cols-[180px_minmax(0,1fr)]">
          <aside className="hidden border-r border-border bg-surface-secondary p-3 md:block">
            <p className="px-2 py-2 text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase">Agent Drives</p>
            {[
              ["Support agent", "headset", true],
              ["Account agent", "presentation", false],
              ["Research agent", "search", false],
            ].map(([label, icon, active]) => (
              <div
                key={String(label)}
                className={`mt-1 flex items-center gap-2 rounded-sm px-2 py-2 text-xs ${
                  active ? "bg-surface font-medium text-foreground shadow-xs" : "text-muted"
                }`}
              >
                <AppIcon icon={String(icon)} size={14} />
                {label}
              </div>
            ))}
            <p className="mt-7 px-2 py-2 text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase">Support folders</p>
            {["customer-uploads", "call-recordings", "renewal-reviews", "incident-notes"].map((folder) => (
              <div key={folder} className="flex items-center gap-2 px-2 py-1.5 text-xs text-muted">
                <AppIcon icon="folder" size={14} />
                <span className="truncate">{folder}</span>
              </div>
            ))}
          </aside>

          <div className="min-w-0">
            <div data-testid="drive-browser-content" className="p-4 md:p-5">
              <div className="flex h-9 items-center gap-2 rounded-sm border border-border bg-background px-3 text-xs text-muted">
                <AppIcon icon="search" size={14} />
                Search files and folders
              </div>
              <Tabs variant="primary" defaultSelectedKey="all" className="mt-4 w-full">
                <Tabs.ListContainer className="max-w-full overflow-x-auto">
                  <Tabs.List aria-label="Choose which files to show" className="w-fit">
                    {filters.map((filter) => (
                      <Tabs.Tab id={filter.id} key={filter.id}>
                        <span className="whitespace-nowrap">{filter.label}</span>
                        <Tabs.Indicator />
                      </Tabs.Tab>
                    ))}
                  </Tabs.List>
                </Tabs.ListContainer>
                {filters.map((filter) => (
                  <Tabs.Panel id={filter.id} key={filter.id} className="p-0 pt-4">
                    <FileRows filter={filter.id} />
                  </Tabs.Panel>
                ))}
              </Tabs>
            </div>
          </div>
        </div>
      </PreviewWindow>
    </MotionConfig>
  )
}

function AttachmentFlowPreview() {
  const stages = [
    {
      label: "Uploaded",
      detail: "return-label-photo.jpg",
      icon: "file-up",
    },
    {
      label: "Analyzed",
      detail: "Shipping label with a cut-off barcode",
      icon: "scan-search",
    },
    {
      label: "Attached",
      detail: "Support agent can inspect it in this session",
      icon: "paperclip",
    },
  ] as const

  return (
    <MotionConfig reducedMotion="user">
      <PreviewWindow title="New session with Support agent">
        <div className="grid min-h-[510px] lg:grid-cols-[1.15fr_0.85fr]">
          <div className="flex flex-col justify-end border-b border-border bg-surface-secondary p-5 md:p-8 lg:border-r lg:border-b-0">
            <div className="mx-auto w-full max-w-[540px]">
              <div className="mb-5 rounded-sm border border-border bg-surface p-4 shadow-xs">
                <div className="flex items-start gap-3">
                  <span className="flex size-11 shrink-0 items-center justify-center rounded-sm bg-surface-secondary">
                    <AppIcon icon="image" size={19} className="text-muted" />
                  </span>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center justify-between gap-3">
                      <p className="truncate text-sm font-medium">return-label-photo.jpg</p>
                      <AppIcon icon="check-circle" size={15} />
                    </div>
                    <p className="mt-1 text-xs text-muted">Image · 2.4 MB · Ready</p>
                  </div>
                </div>
              </div>
              <div className="rounded-sm border border-border bg-surface p-4 shadow-sm">
                <p className="min-h-16 text-sm leading-6 text-foreground/80">
                  Find out why this return label won’t scan and tell me what to send the customer.
                </p>
                <div className="mt-3 flex items-center justify-between border-t border-border pt-3 text-muted">
                  <span className="flex items-center gap-3">
                    <AppIcon icon="paperclip" size={16} />
                    <AppIcon icon="mic" size={16} />
                  </span>
                  <span className="flex size-8 items-center justify-center rounded-sm bg-foreground text-background">
                    <AppIcon icon="arrow-up" size={15} />
                  </span>
                </div>
              </div>
            </div>
          </div>

          <div className="flex flex-col justify-center p-6 md:p-9">
            <p className="text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase">Before the message is sent</p>
            <div className="mt-7">
              {stages.map((stage, index) => (
                <motion.div
                  key={stage.label}
                  initial={{ opacity: 0, x: 8 }}
                  whileInView={{ opacity: 1, x: 0 }}
                  viewport={{ once: true, amount: 0.65 }}
                  transition={{
                    delay: index * 0.12,
                    duration: 0.42,
                    ease: easeOut,
                  }}
                  className="grid grid-cols-[36px_1fr] gap-3 border-t border-border py-5"
                >
                  <span className="flex size-8 items-center justify-center rounded-sm bg-surface-secondary text-muted">
                    <AppIcon icon={stage.icon} size={15} />
                  </span>
                  <div>
                    <p className="text-sm font-medium">{stage.label}</p>
                    <p className="mt-1 text-xs leading-5 text-muted">{stage.detail}</p>
                  </div>
                </motion.div>
              ))}
            </div>
          </div>
        </div>
      </PreviewWindow>
    </MotionConfig>
  )
}

function AgentOutputPreview() {
  return (
    <PreviewWindow title="Revenue agent / Session 2841">
      <div className="grid min-h-[490px] lg:grid-cols-[0.8fr_1.2fr]">
        <div className="border-b border-border p-6 md:p-9 lg:border-r lg:border-b-0">
          <p className="text-xs text-muted">You</p>
          <p className="mt-2 text-sm leading-6">Review the account notes, rank renewal risk, and give me a file I can use in Monday’s meeting.</p>
          <div className="mt-8 border-t border-border pt-6">
            <div className="flex items-center gap-2 text-xs text-muted">
              <span className="flex size-7 items-center justify-center rounded-sm bg-surface-secondary">
                <AppIcon icon="bot" size={14} />
              </span>
              Revenue agent
            </div>
            <p className="mt-4 text-sm leading-6">
              I ranked the accounts and saved the review files to Drive. The CSV has one row per account; the brief explains the top risks and next actions.
            </p>
          </div>
        </div>
        <div className="flex flex-col justify-center bg-surface-secondary p-6 md:p-10">
          <div className="mx-auto w-full max-w-[540px] space-y-3">
            {[
              {
                name: "renewal-risk.csv",
                folder: "account-reviews/2026-q3",
                detail: "24 rows · 84 KB",
                icon: "table",
              },
              {
                name: "renewal-brief.md",
                folder: "account-reviews/2026-q3",
                detail: "5 sections · 18 KB",
                icon: "file-text",
              },
            ].map((file, index) => (
              <motion.div
                key={file.name}
                initial={{ opacity: 0, y: 9 }}
                whileInView={{ opacity: 1, y: 0 }}
                viewport={{ once: true, amount: 0.65 }}
                transition={{
                  delay: 0.12 + index * 0.1,
                  duration: 0.4,
                  ease: easeOut,
                }}
                className="flex items-center gap-4 rounded-sm border border-border bg-surface p-4 shadow-xs"
              >
                <span className="flex size-10 shrink-0 items-center justify-center rounded-sm bg-surface-secondary text-muted">
                  <AppIcon icon={file.icon} size={18} />
                </span>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{file.name}</p>
                  <p className="mt-1 truncate text-xs text-muted">{file.folder}</p>
                </div>
                <span className="hidden text-xs text-muted sm:block">{file.detail}</span>
                <AppIcon icon="download" size={15} className="text-muted" />
              </motion.div>
            ))}
            <p className="px-1 pt-2 text-xs leading-5 text-muted">Saved by the agent from its sandbox with Drive upload.</p>
          </div>
        </div>
      </div>
    </PreviewWindow>
  )
}

function MetadataPreview() {
  return (
    <PreviewWindow title="return-label-photo.jpg">
      <div className="grid min-h-[500px] md:grid-cols-[1.1fr_0.9fr]">
        <div className="relative flex min-h-72 items-center justify-center border-b border-border bg-surface-secondary p-8 md:border-r md:border-b-0">
          <div
            role="img"
            aria-label="Uploaded return label image placeholder"
            className="flex aspect-[4/3] w-full max-w-[470px] items-center justify-center rounded-sm border border-dashed border-border bg-surface text-center"
          >
            <div>
              <AppIcon icon="image" size={24} className="mx-auto text-muted" />
              <p className="mt-3 text-xs text-muted">Uploaded image placeholder</p>
            </div>
          </div>
        </div>
        <div className="p-6 md:p-9">
          <p className="text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase">File details</p>
          <dl className="mt-5">
            {[
              ["Type", "image/jpeg"],
              ["Size", "2.4 MB"],
              ["Folder", "uploads"],
              ["Agent", "Support agent"],
            ].map(([label, value]) => (
              <div key={label} className="flex items-center justify-between gap-6 border-t border-border py-3.5 text-xs">
                <dt className="text-muted">{label}</dt>
                <dd className="text-right font-medium">{value}</dd>
              </div>
            ))}
          </dl>
          <div className="mt-7 rounded-sm bg-surface-secondary p-5">
            <div className="flex items-center gap-2 text-xs font-medium">
              <AppIcon icon="scan-search" size={15} /> Image description
            </div>
            <p className="mt-3 text-sm leading-6 text-muted">
              A return shipping label photographed on a desk. The barcode is cut off at the right edge, which may prevent a complete scan.
            </p>
          </div>
        </div>
      </div>
    </PreviewWindow>
  )
}

export function SearchDownloadPreview() {
  return (
    <PreviewWindow title="Revenue agent / Session files">
      <div className="grid min-h-[450px] lg:grid-cols-[1fr_auto_1fr]">
        <div className="p-6 md:p-9">
          <div className="flex items-center justify-between gap-4">
            <p className="text-xs font-medium">Find the source file</p>
            <span className="font-mono text-[0.65rem] text-muted">drive_search</span>
          </div>
          <div className="mt-5 rounded-sm border border-border bg-background p-4 font-mono text-[0.68rem] leading-5">
            <span className="text-muted">q:</span> at-risk renewals
            <br />
            <span className="text-muted">extension:</span> csv
            <br />
            <span className="text-muted">path_prefix:</span> reviews
          </div>
          <div className="mt-4 rounded-sm border border-border p-4">
            <div className="flex items-start gap-3">
              <AppIcon icon="table" size={17} className="mt-0.5 text-muted" />
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">accounts-at-risk.csv</p>
                <p className="mt-1 text-xs text-muted">reviews/2026-q3 · 84 KB</p>
                <p className="mt-3 font-mono text-[0.62rem] text-muted">asset_id: 8c1f...4a2b</p>
              </div>
            </div>
          </div>
        </div>
        <div className="hidden items-center px-2 text-muted lg:flex">
          <span className="h-px w-8 bg-border" />
          <AppIcon icon="arrow-right" size={15} />
          <span className="h-px w-8 bg-border" />
        </div>
        <div className="border-t border-border bg-surface-secondary p-6 md:p-9 lg:border-t-0 lg:border-l">
          <div className="flex items-center justify-between gap-4">
            <p className="text-xs font-medium">Add it to this session</p>
            <span className="font-mono text-[0.65rem] text-muted">drive_download</span>
          </div>
          <div className="mt-5 overflow-hidden rounded-sm border border-border bg-foreground font-mono text-background">
            <div className="border-b border-background/15 px-4 py-3 text-[0.65rem] text-background/60">/workspace/input</div>
            <div className="p-4 text-[0.7rem] leading-6">
              <p className="text-background/55">$ ls</p>
              <p>accounts-at-risk.csv</p>
              <p className="mt-3 text-background/55">$ wc -l accounts-at-risk.csv</p>
              <p>25 accounts-at-risk.csv</p>
            </div>
          </div>
          <p className="mt-4 text-xs leading-5 text-muted">The asset ID links search to download, so the session gets the file the agent selected.</p>
        </div>
      </div>
    </PreviewWindow>
  )
}

function FolderMovePreview() {
  return (
    <PreviewWindow title="Revenue agent / Drive">
      <div className="min-h-[430px] p-6 md:p-10">
        <div className="mx-auto max-w-[760px]">
          <div className="grid gap-5 md:grid-cols-[1fr_auto_1fr] md:items-center">
            <div className="rounded-sm border border-border bg-surface-secondary p-5">
              <p className="text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase">Before review</p>
              <div className="mt-8 flex items-center gap-3">
                <AppIcon icon="folder" size={18} className="text-muted" />
                <div>
                  <p className="text-sm font-medium">drafts</p>
                  <p className="mt-1 text-xs text-muted">renewal-brief.md</p>
                </div>
              </div>
            </div>
            <div className="flex justify-center text-muted">
              <AppIcon icon="move" size={18} />
            </div>
            <div className="rounded-sm border border-border bg-surface p-5 shadow-xs">
              <div className="flex items-center justify-between gap-3">
                <p className="text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase">Ready for Monday</p>
                <AppIcon icon="check-circle" size={15} />
              </div>
              <div className="mt-8 flex items-center gap-3">
                <AppIcon icon="folder-open" size={18} className="text-muted" />
                <div>
                  <p className="text-sm font-medium">account-reviews/2026-q3</p>
                  <p className="mt-1 text-xs text-muted">renewal-brief.md</p>
                </div>
              </div>
            </div>
          </div>
          <div className="mt-9 grid gap-3 border-t border-border pt-6 text-xs md:grid-cols-2">
            <p className="flex items-center gap-2 text-muted">
              <AppIcon icon="link" size={14} /> Asset address stays the same
            </p>
            <p className="flex items-center gap-2 text-muted md:justify-end">
              <AppIcon icon="history" size={14} /> Folder and updated time change
            </p>
          </div>
        </div>
      </div>
    </PreviewWindow>
  )
}

function AccessBoundaryPreview() {
  const agents = [
    {
      name: "Support agent",
      folder: "uploads, incident-briefs",
      icon: "headset",
    },
    {
      name: "Revenue agent",
      folder: "account-reviews",
      icon: "presentation",
    },
    {
      name: "Research agent",
      folder: "sources, reports",
      icon: "search",
    },
  ] as const

  return (
    <div className="overflow-hidden rounded-sm border border-border bg-surface">
      <div className="grid lg:grid-cols-[0.76fr_1.24fr]">
        <div className="flex flex-col justify-between border-b border-border p-7 md:p-11 lg:border-r lg:border-b-0">
          <div>
            <div className="flex size-10 items-center justify-center rounded-sm bg-surface-secondary">
              <AppIcon icon="shield-check" size={19} />
            </div>
            <p className="mt-8 text-sm font-medium">One org catalog</p>
            <p className="mt-3 max-w-[38ch] text-sm leading-6 text-muted">
              Hivy keeps each asset tied to the organization and agent that owns it. Runtime credentials must match that agent and sandbox.
            </p>
          </div>
          <p className="mt-12 text-xs leading-5 text-muted">Parent-agent Drive files aren’t exposed to sub-agents.</p>
        </div>
        <div className="bg-surface-secondary p-5 md:p-8 lg:p-11">
          <div className="rounded-sm border border-border bg-surface">
            <div className="flex items-center justify-between border-b border-border px-5 py-4">
              <p className="text-sm font-medium">Northstar workspace</p>
              <span className="text-xs text-muted">Org-scoped</span>
            </div>
            {agents.map((agent, index) => (
              <div
                key={agent.name}
                className={`grid gap-4 px-5 py-5 sm:grid-cols-[1fr_auto] sm:items-center ${index < agents.length - 1 ? "border-b border-border" : ""}`}
              >
                <div className="flex items-center gap-3">
                  <span className="flex size-9 items-center justify-center rounded-sm bg-surface-secondary text-muted">
                    <AppIcon icon={agent.icon} size={16} />
                  </span>
                  <div>
                    <p className="text-sm font-medium">{agent.name}</p>
                    <p className="mt-1 text-xs text-muted">{agent.folder}</p>
                  </div>
                </div>
                <span className="inline-flex w-fit items-center gap-1.5 rounded-sm border border-border px-2.5 py-1.5 text-[0.68rem] text-muted">
                  <AppIcon icon="key-round" size={12} /> Own Drive only
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
