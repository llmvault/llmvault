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

function PreviewWindow({
  title,
  action,
  children,
  className = "",
}: {
  title: string
  action?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <div
      className={`overflow-hidden rounded-sm border border-border bg-surface shadow-xs ${className}`}
    >
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
  const visibleFiles =
    filter === "all" ? files : files.filter((file) => file.type === filter)

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
              <p className="mt-0.5 truncate text-[0.7rem] text-muted">
                {file.owner}
              </p>
            </div>
          </div>
          <span className="hidden truncate text-xs text-muted md:block">
            {file.path}
          </span>
          <span className="hidden text-xs text-muted md:block">
            {file.size}
          </span>
          <span className="hidden text-xs text-muted md:block">
            {file.updated}
          </span>
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
            <p className="px-2 py-2 text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase">
              Agent Drives
            </p>
            {[
              ["Support agent", "headset", true],
              ["Account agent", "presentation", false],
              ["Research agent", "search", false],
            ].map(([label, icon, active]) => (
              <div
                key={String(label)}
                className={`mt-1 flex items-center gap-2 rounded-sm px-2 py-2 text-xs ${
                  active
                    ? "bg-surface font-medium text-foreground shadow-xs"
                    : "text-muted"
                }`}
              >
                <AppIcon icon={String(icon)} size={14} />
                {label}
              </div>
            ))}
            <p className="mt-7 px-2 py-2 text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase">
              Support folders
            </p>
            {[
              "customer-uploads",
              "call-recordings",
              "renewal-reviews",
              "incident-notes",
            ].map((folder) => (
              <div
                key={folder}
                className="flex items-center gap-2 px-2 py-1.5 text-xs text-muted"
              >
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
              <Tabs
                variant="primary"
                defaultSelectedKey="all"
                className="mt-4 w-full"
              >
                <Tabs.ListContainer className="max-w-full overflow-x-auto">
                  <Tabs.List
                    aria-label="Choose which files to show"
                    className="w-fit"
                  >
                    {filters.map((filter) => (
                      <Tabs.Tab id={filter.id} key={filter.id}>
                        <span className="whitespace-nowrap">
                          {filter.label}
                        </span>
                        <Tabs.Indicator />
                      </Tabs.Tab>
                    ))}
                  </Tabs.List>
                </Tabs.ListContainer>
                {filters.map((filter) => (
                  <Tabs.Panel
                    id={filter.id}
                    key={filter.id}
                    className="p-0 pt-4"
                  >
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

export function SearchDownloadPreview() {
  return (
    <PreviewWindow title="Revenue agent / Session files">
      <div className="grid min-h-[450px] lg:grid-cols-[1fr_auto_1fr]">
        <div className="p-6 md:p-9">
          <div className="flex items-center justify-between gap-4">
            <p className="text-xs font-medium">Find the source file</p>
            <span className="font-mono text-[0.65rem] text-muted">
              drive_search
            </span>
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
                <p className="truncate text-sm font-medium">
                  accounts-at-risk.csv
                </p>
                <p className="mt-1 text-xs text-muted">
                  reviews/2026-q3 · 84 KB
                </p>
                <p className="mt-3 font-mono text-[0.62rem] text-muted">
                  asset_id: 8c1f...4a2b
                </p>
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
            <span className="font-mono text-[0.65rem] text-muted">
              drive_download
            </span>
          </div>
          <div className="mt-5 overflow-hidden rounded-sm border border-border bg-foreground font-mono text-background">
            <div className="border-b border-background/15 px-4 py-3 text-[0.65rem] text-background/60">
              /workspace/input
            </div>
            <div className="p-4 text-[0.7rem] leading-6">
              <p className="text-background/55">$ ls</p>
              <p>accounts-at-risk.csv</p>
              <p className="mt-3 text-background/55">
                $ wc -l accounts-at-risk.csv
              </p>
              <p>25 accounts-at-risk.csv</p>
            </div>
          </div>
          <p className="mt-4 text-xs leading-5 text-muted">
            The asset ID links search to download, so the session gets the file
            the agent selected.
          </p>
        </div>
      </div>
    </PreviewWindow>
  )
}
