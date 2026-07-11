"use client"

import { useMemo, useState } from "react"
import { Popover } from "@heroui/react"
import ScrollToBottom from "react-scroll-to-bottom"
import { AppIcon } from "@/components/icon"
import { Conversation } from "@/app/w/(chat)/_components/conversation"
import { sessionEventsToConversationBlocks } from "@/app/w/(chat)/_lib/session-history"
import type { SessionSubagentRun } from "@/app/w/(chat)/_lib/session-subagent-runs"
import { useSessionSubagentRuns } from "@/app/w/(chat)/_stores/session-runtime-store"
import {
  selectSessionWorkspace,
  useSessionWorkspaceStore,
} from "@/app/w/(chat)/_stores/session-workspace-store"
import {
  subagentRunOptions,
  subagentRunStatus,
  subagentRunTitle,
} from "./subagent-run-options"
import { useSubagentHistory } from "./use-subagent-history"

const followButtonClassName =
  "!absolute !bottom-6 !left-1/2 !right-auto !flex !h-9 !w-9 !-translate-x-1/2 !items-center !justify-center !rounded-full !border !border-border !bg-surface !p-0 !text-muted !shadow-sm !transition-colors after:content-['↓'] hover:!bg-default hover:!text-foreground"

export function SubagentView({ sessionId }: { sessionId?: string }) {
  const workspaceSessionId = sessionId ?? "new-chat"
  const activeJobId = useSessionWorkspaceStore(
    (state) =>
      selectSessionWorkspace(state, workspaceSessionId).subagents.activeJobId
  )
  const openSubagentRun = useSessionWorkspaceStore(
    (state) => state.openSubagentRun
  )
  const runs = useSessionSubagentRuns(sessionId)
  // `activeJobId` may hold either a jobId or a childSessionId. Match on both.
  // Only fall back to the first run when nothing is selected at all; when a
  // selection is present but not yet matched, render a distinct waiting state
  // instead of silently resolving to a different (wrong) run.
  const selectedRun = activeJobId
    ? runs.find(
        (item) =>
          item.jobId === activeJobId || item.childSessionId === activeJobId
      )
    : undefined
  const run = selectedRun ?? (activeJobId ? undefined : runs[0])
  const history = useSubagentHistory(sessionId, run)
  const blocks = useMemo(
    () =>
      sessionEventsToConversationBlocks(history.events, {
        mode: run?.status === "running" ? "live" : "history",
      }),
    [history.events, run?.status]
  )

  if (!run) {
    if (activeJobId) {
      return (
        <SubagentEmptyState
          title="Waiting for subagent"
          message="Waiting for this subagent's stream to arrive."
        />
      )
    }
    return (
      <SubagentEmptyState
        title="No subagent selected"
        message="Open a subagent card from the conversation to inspect its stream."
      />
    )
  }

  return (
    <div className="flex h-full min-w-0 flex-col">
      <div className="shrink-0 border-b border-border px-4 py-3">
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-default">
            <AppIcon icon={statusIcon(run)} className="h-4 w-4 text-muted" />
          </div>
          <SubagentRunSelector
            runs={runs}
            selectedRun={run}
            onSelect={(jobId) => openSubagentRun(workspaceSessionId, jobId)}
          />
        </div>
      </div>

      {history.failed ? (
        <div className="flex shrink-0 items-center justify-between gap-3 border-b border-border bg-danger/5 px-4 py-2 text-xs text-danger">
          <span>Could not load the complete subagent history.</span>
          <button
            type="button"
            className="shrink-0 font-medium underline underline-offset-2"
            onClick={() => void history.retry()}
          >
            Retry
          </button>
        </div>
      ) : null}

      <ScrollToBottom
        key={run.jobId}
        className="min-h-0 flex-1"
        scrollViewClassName="min-h-0 [overflow-anchor:none]"
        followButtonClassName={followButtonClassName}
        initialScrollBehavior="auto"
        mode="bottom"
      >
        {blocks.length ? (
          <Conversation blocks={blocks} />
        ) : (
          <SubagentEmptyState
            title={
              run.status === "running" ? "Waiting for stream" : "No output"
            }
            message={emptyRunMessage(run)}
          />
        )}
      </ScrollToBottom>
    </div>
  )
}

function SubagentRunSelector({
  runs,
  selectedRun,
  onSelect,
}: {
  runs: SessionSubagentRun[]
  selectedRun: SessionSubagentRun
  onSelect: (jobId: string) => void
}) {
  const [open, setOpen] = useState(false)
  const options = subagentRunOptions(runs)
  const selected = options.find((option) => option.id === selectedRun.jobId)

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label={`Switch subagent, ${selected?.identifier ?? selectedRun.jobId} selected`}
        className="group flex min-w-0 flex-1 items-center gap-2 rounded-lg px-2 py-1 text-left transition-colors hover:bg-default"
      >
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium text-foreground">
            {selected?.label ?? subagentRunTitle(selectedRun)}
          </span>
          <span className="block truncate text-xs text-muted">
            {subagentRunStatus(selectedRun)}
            {` · ${selected?.identifier ?? selectedRun.jobId}`}
          </span>
        </span>
        <AppIcon
          icon="chevron-down"
          className="h-3.5 w-3.5 shrink-0 text-muted transition-colors group-hover:text-foreground"
        />
      </Popover.Trigger>
      <Popover.Content className="w-80 max-w-[calc(100vw-2rem)] border border-border p-1.5">
        <Popover.Dialog
          aria-label="Subagent runs"
          className="flex w-full flex-col gap-0.5 p-0"
        >
          {options.map((option) => {
            const selectedOption = option.id === selectedRun.jobId
            return (
              <button
                key={option.id}
                type="button"
                aria-current={selectedOption ? "true" : undefined}
                className={`focus-visible:outline-primary flex w-full min-w-0 items-center gap-3 rounded-lg px-3 py-2 text-left transition-colors hover:bg-default focus-visible:outline-2 focus-visible:outline-offset-1 ${
                  selectedOption ? "bg-default" : ""
                }`}
                onClick={() => {
                  onSelect(option.id)
                  setOpen(false)
                }}
              >
                <AppIcon
                  icon={statusIcon(
                    runs.find((item) => item.jobId === option.id) ?? selectedRun
                  )}
                  className={`h-4 w-4 shrink-0 text-muted ${
                    option.status === "Running" ? "animate-spin" : ""
                  }`}
                />
                <span className="min-w-0 flex-1">
                  <span className="flex min-w-0 items-center gap-2">
                    <span className="truncate text-sm font-medium text-foreground">
                      {option.label}
                    </span>
                    <span className="shrink-0 text-xs text-muted">
                      {option.status}
                    </span>
                  </span>
                  <span
                    className="mt-0.5 block truncate font-mono text-[11px] text-muted"
                    title={option.identifier}
                  >
                    {option.identifier}
                  </span>
                  <span className="mt-0.5 block truncate text-xs text-muted">
                    {option.detail === option.identifier ? "Subagent run" : option.detail}
                  </span>
                </span>
                {selectedOption ? (
                  <AppIcon
                    icon="check"
                    className="text-primary h-4 w-4 shrink-0"
                  />
                ) : null}
              </button>
            )
          })}
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
}

function SubagentEmptyState({
  title,
  message,
}: {
  title: string
  message: string
}) {
  return (
    <div className="flex h-full items-center justify-center px-6 text-center">
      <div className="max-w-sm">
        <div className="mx-auto flex h-10 w-10 items-center justify-center rounded-xl bg-default">
          <AppIcon icon="bot" className="h-5 w-5 text-muted" />
        </div>
        <h3 className="mt-3 text-sm font-medium text-foreground">{title}</h3>
        <p className="mt-1 text-sm leading-6 text-muted">{message}</p>
      </div>
    </div>
  )
}

function statusIcon(run: SessionSubagentRun) {
  if (run.status === "completed") return "check"
  if (run.status === "failed") return "triangle-alert"
  return "loader-circle"
}

function emptyRunMessage(run: SessionSubagentRun) {
  if (run.error) return run.error
  if (run.status === "running") {
    return "This subagent has not emitted visible conversation events yet."
  }
  return "This subagent completed without visible conversation events."
}
