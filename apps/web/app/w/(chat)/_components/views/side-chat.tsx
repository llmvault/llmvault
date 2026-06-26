"use client"

import { useMemo } from "react"
import { Icon } from "@iconify/react"
import { Conversation } from "@/app/w/(chat)/_components/conversation"
import { sessionEventsToConversationBlocks } from "@/app/w/(chat)/_lib/session-history"
import type { SessionSubagentRun } from "@/app/w/(chat)/_lib/session-subagent-runs"
import { useSessionSubagentRuns } from "@/app/w/(chat)/_stores/session-runtime-store"
import {
  selectSessionWorkspace,
  useSessionWorkspaceStore,
} from "@/app/w/(chat)/_stores/session-workspace-store"

export function SubagentView({ sessionId }: { sessionId?: string }) {
  const workspaceSessionId = sessionId ?? "new-chat"
  const activeJobId = useSessionWorkspaceStore(
    (state) =>
      selectSessionWorkspace(state, workspaceSessionId).subagents.activeJobId
  )
  const runs = useSessionSubagentRuns(sessionId)
  const run = runs.find((item) => item.jobId === activeJobId) ?? runs[0]
  const blocks = useMemo(
    () =>
      sessionEventsToConversationBlocks(run?.events ?? [], {
        mode: run?.status === "running" ? "live" : "history",
      }),
    [run?.events, run?.status]
  )

  if (!run) {
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
            <Icon icon={statusIcon(run)} className="h-4 w-4 text-muted" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium text-foreground">
              {subagentTitle(run)}
            </div>
            <div className="truncate text-xs text-muted">
              {statusLabel(run)}
              {run.childSessionId ? ` / ${run.childSessionId}` : ""}
            </div>
          </div>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
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
      </div>
    </div>
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
          <Icon icon="lucide:bot" className="h-5 w-5 text-muted" />
        </div>
        <h3 className="mt-3 text-sm font-medium text-foreground">{title}</h3>
        <p className="mt-1 text-sm leading-6 text-muted">{message}</p>
      </div>
    </div>
  )
}

function subagentTitle(run: SessionSubagentRun) {
  return run.agentName?.trim() || "Subagent"
}

function statusLabel(run: SessionSubagentRun) {
  if (run.status === "completed") return "Completed"
  if (run.status === "failed")
    return run.error ? `Failed: ${run.error}` : "Failed"
  return "Running"
}

function statusIcon(run: SessionSubagentRun) {
  if (run.status === "completed") return "lucide:check"
  if (run.status === "failed") return "lucide:triangle-alert"
  return "lucide:loader-circle"
}

function emptyRunMessage(run: SessionSubagentRun) {
  if (run.error) return run.error
  if (run.status === "running") {
    return "This subagent has not emitted visible conversation events yet."
  }
  return "This subagent completed without visible conversation events."
}
