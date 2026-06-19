"use client"

import { AnimatePresence, motion } from "motion/react"
import { Icon } from "@iconify/react"
import { Link, Spinner } from "@heroui/react"
import { useState } from "react"
import { CommentablePatchDiff } from "@/app/w/(chat)/_components/diff-line-comments"
import type {
  ConversationBlock,
  ToolCallDetail,
} from "@/app/w/(chat)/_lib/static-data"
import { HIVY_DIFF_STYLE, hivyDiffOptions } from "@/lib/diffs-theme"

const COLLAPSE_TRANSITION = {
  duration: 0.25,
  ease: [0.32, 0.72, 0, 1] as const,
}

const TOOL_DIFF_OPTIONS = hivyDiffOptions({
  diffStyle: "unified",
})

export function ToolBlock({
  block,
}: {
  block: Extract<ConversationBlock, { type: "tool" }>
}) {
  const [expanded, setExpanded] = useState(false)
  const expandable = block.detail ? hasExpandableDetail(block.detail) : false

  if (!block.detail) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted">
        <Icon
          icon="lucide:square-chevron-right"
          className="h-3.5 w-3.5 shrink-0"
        />
        <span
          className={`min-w-0 flex-1 truncate font-mono text-sm ${
            block.running ? "hivy-shimmer" : ""
          }`}
        >
          {block.label}
        </span>
      </div>
    )
  }

  return (
    <div className="flex min-w-0 flex-col gap-2">
      <button
        type="button"
        onClick={() => {
          if (expandable) setExpanded((open) => !open)
        }}
        className="focus-visible:outline-warning flex w-full min-w-0 items-center gap-2 text-left text-sm text-muted transition-colors hover:text-foreground focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2"
        aria-expanded={expandable ? expanded : undefined}
      >
        <ToolIcon detail={block.detail} />
        <span
          className={`min-w-0 flex-1 truncate text-sm ${
            block.running ? "hivy-shimmer" : ""
          }`}
        >
          {block.label}
        </span>
        {expandable ? (
          <Icon
            icon="lucide:chevron-down"
            className={`h-4 w-4 shrink-0 transition-transform ${expanded ? "rotate-180" : ""}`}
          />
        ) : null}
      </button>

      <Collapse open={expanded}>
        {expandable ? (
          <ToolDetail detail={block.detail} running={block.running} />
        ) : null}
      </Collapse>
    </div>
  )
}

function ToolDetail({
  detail,
  running,
}: {
  detail: ToolCallDetail
  running?: boolean
}) {
  if (detail.category === "file_read") {
    return <ReadFileDetail detail={detail} />
  }

  if (detail.category === "file_edit" || detail.category === "file_write") {
    return <FileMutationDetail detail={detail} />
  }

  if (detail.category === "web_search" && detail.searchResults?.length) {
    return <WebSearchDetail detail={detail} running={running} />
  }

  const body = toolBody(detail)
  return (
    <div className="bg-default rounded-2xl px-4 py-3 text-muted">
      <div className="text-sm">{detail.kind}</div>
      <pre className="mt-7 font-mono text-sm leading-6 break-words whitespace-pre-wrap text-foreground">
        {body}
      </pre>
      {detail.truncated ? (
        <div className="mt-3 text-sm">Output truncated</div>
      ) : null}
      <div className="mt-8 flex items-center justify-end gap-1.5 text-sm">
        {running ? (
          <Spinner color="current" size="sm" />
        ) : (
          <Icon
            icon={toolFailed(detail) ? "lucide:triangle-alert" : "lucide:check"}
            className="h-4 w-4"
          />
        )}
        <span>{statusLabel(detail, running)}</span>
      </div>
    </div>
  )
}

function WebSearchDetail({
  detail,
}: {
  detail: ToolCallDetail
  running?: boolean
}) {
  const results = detail.searchResults ?? []

  return (
    <div className="flex min-w-0 flex-col gap-1 text-sm">
      {results.map((result, index) => (
        <Link
          key={`${result.url}-${index}`}
          href={result.url}
          target="_blank"
          rel="noreferrer"
          className="block min-w-0 truncate font-mono text-[13px] text-muted underline-offset-2 hover:text-foreground hover:underline"
        >
          {result.url}
        </Link>
      ))}
    </div>
  )
}

function ReadFileDetail({ detail }: { detail: ToolCallDetail }) {
  const files = detail.paths?.length
    ? detail.paths
    : detail.path
      ? [detail.path]
      : []

  return (
    <div className="flex flex-col gap-2 pt-1 text-sm text-muted">
      {files.length > 0 ? (
        files.map((file) => (
          <div key={file} className="min-w-0 truncate">
            Read{" "}
            <span className="font-mono text-[14px]">
              {displayFileName(file)}
            </span>
          </div>
        ))
      ) : (
        <div>Read a file</div>
      )}
    </div>
  )
}

function FileMutationDetail({ detail }: { detail: ToolCallDetail }) {
  const patches = detail.diff ? splitPatches(detail.diff) : []
  const file = detail.path ?? detail.paths?.[0]
  const title = detail.category === "file_write" ? "Wrote file" : "Edited file"

  return (
    <div className="flex min-w-0 flex-col gap-3">
      <div className="flex items-center gap-2 text-sm text-muted">
        <span>{title}</span>
        {file ? (
          <span className="min-w-0 truncate font-mono text-[14px]">
            {displayFileName(file)}
          </span>
        ) : null}
        {typeof detail.editsApplied === "number" ? (
          <span>{detail.editsApplied} edits</span>
        ) : null}
        {typeof detail.bytesWritten === "number" ? (
          <span>{formatBytes(detail.bytesWritten)}</span>
        ) : null}
      </div>

      {patches.length > 0 ? (
        <div className="flex min-w-0 flex-col gap-3 overflow-hidden rounded-2xl border border-border bg-background">
          {patches.map((patch, index) => (
            <CommentablePatchDiff
              key={`${index}-${patch.slice(0, 32)}`}
              patch={patch}
              options={TOOL_DIFF_OPTIONS}
              source={{
                kind: "tool",
                path: file,
              }}
              style={HIVY_DIFF_STYLE}
              disableWorkerPool
            />
          ))}
        </div>
      ) : file ? (
        <div className="bg-default rounded-2xl px-4 py-3 font-mono text-[14px] text-muted">
          {file}
        </div>
      ) : null}
    </div>
  )
}

function Collapse({
  open,
  children,
}: {
  open: boolean
  children: React.ReactNode
}) {
  return (
    <AnimatePresence initial={false}>
      {open ? (
        <motion.div
          initial={{ height: 0, opacity: 0 }}
          animate={{ height: "auto", opacity: 1 }}
          exit={{ height: 0, opacity: 0 }}
          transition={COLLAPSE_TRANSITION}
          className="overflow-hidden"
        >
          {children}
        </motion.div>
      ) : null}
    </AnimatePresence>
  )
}

function toolBody(detail: ToolCallDetail) {
  const sections: string[] = []
  if (detail.command) sections.push(`$ ${detail.command}`)
  if (!detail.command && detail.url) sections.push(detail.url)
  if (!detail.command && !detail.url && detail.input)
    sections.push(detail.input)
  if (detail.output) sections.push(detail.output.trimEnd())
  if (detail.error) sections.push(detail.error)
  if (sections.length === 0) return "No output"
  return sections.join("\n")
}

function toolFailed(detail: ToolCallDetail) {
  return (
    detail.status === "errored" ||
    detail.timedOut === true ||
    Boolean(detail.error) ||
    (typeof detail.exitCode === "number" && detail.exitCode !== 0)
  )
}

function statusLabel(detail: ToolCallDetail, running?: boolean) {
  if (running) return "Running"
  if (detail.timedOut) return "Timed out"
  if (toolFailed(detail)) return "Failed"
  return "Success"
}

function toolIcon(detail: ToolCallDetail) {
  return detail.icon || "lucide:square-chevron-right"
}

function ToolIcon({ detail }: { detail: ToolCallDetail }) {
  const icon = toolIcon(detail)
  return (
    <Icon
      icon={icon}
      className={`h-4 w-4 shrink-0 ${
        icon === "logos:chrome" ? "" : "text-muted"
      }`}
    />
  )
}

function hasExpandableDetail(detail: ToolCallDetail) {
  if (detail.category === "web_fetch") {
    return false
  }
  if (detail.category === "web_search") {
    return Boolean(detail.searchResults?.length || detail.error)
  }
  if (detail.category === "file_read") {
    return Boolean(detail.path || detail.paths?.length)
  }
  if (detail.category === "file_edit" || detail.category === "file_write") {
    return true
  }
  return Boolean(detail.command || detail.output || detail.error)
}

function splitPatches(diff: string) {
  const clean = diff.trim()
  if (!clean) return []
  const patches = clean.includes("\ndiff --git ")
    ? clean.split(/(?=^diff --git )/m)
    : [clean]
  return patches.map((patch) => `${patch.trimEnd()}\n`).filter(Boolean)
}

function displayFileName(path: string) {
  const normalized = path.replace(/\\/g, "/")
  return normalized.split("/").filter(Boolean).pop() || path
}

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 102.4) / 10} KB`
  return `${Math.round(bytes / 1024 / 102.4) / 10} MB`
}
