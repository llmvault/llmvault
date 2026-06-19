"use client"

import { useEffect, useRef, type CSSProperties } from "react"
import type { GitStatusEntry } from "@pierre/trees"
import { FileTree, useFileTree } from "@pierre/trees/react"
import { Button, Spinner } from "@heroui/react"
import { Icon } from "@iconify/react"

type TreeCSSProperties = CSSProperties & Record<`--${string}`, string | number>

const TREE_THEME_STYLE: TreeCSSProperties = {
  height: "100%",
  minHeight: 0,
  width: "100%",
  "--trees-bg-override": "var(--surface)",
  "--trees-bg-muted-override": "var(--surface-secondary)",
  "--trees-fg-override": "var(--surface-foreground)",
  "--trees-fg-muted-override": "var(--muted)",
  "--trees-border-color-override": "var(--border)",
  "--trees-focus-ring-color-override": "var(--focus)",
  "--trees-selected-bg-override": "var(--surface-secondary)",
  "--trees-selected-fg-override": "var(--surface-foreground)",
  "--trees-selected-focused-border-color-override": "var(--border)",
  "--trees-indent-guide-bg-override": "var(--border)",
  "--trees-scrollbar-thumb-override": "var(--scrollbar)",
  "--trees-font-family-override": "var(--font-sans)",
  "--trees-font-size-override": "13px",
  "--trees-border-radius-override": "8px",
  "--trees-item-padding-x-override": "6px",
  "--trees-item-margin-x-override": "4px",
  "--trees-git-added-color-override": "var(--success)",
  "--trees-git-deleted-color-override": "var(--danger)",
  "--trees-git-modified-color-override": "var(--warning)",
  "--trees-git-renamed-color-override": "var(--accent)",
  "--trees-git-untracked-color-override": "var(--muted)",
}

const TREE_UNSAFE_CSS = `
  :host {
    background: var(--surface);
  }

  [data-type='item'] {
    border-radius: 8px;
    transition:
      background-color 140ms ease,
      color 140ms ease;
  }

  [data-type='item']:hover {
    background: var(--surface-secondary);
  }

  [data-type='item'][data-item-selected] {
    background: var(--surface-secondary);
  }

  [data-type='item'][data-item-focused] {
    outline-color: var(--focus);
  }
`

export function RuntimeFileTree({
  directoryPaths,
  paths,
  gitStatus,
  loadedDirectoryPaths,
  loadingDirectoryPaths,
  selectedPath,
  onDirectoryExpand,
  onSelectPath,
}: {
  directoryPaths: string[]
  paths: string[]
  gitStatus: GitStatusEntry[]
  loadedDirectoryPaths: Set<string>
  loadingDirectoryPaths: Set<string>
  selectedPath: string | null
  onDirectoryExpand: (path: string) => void
  onSelectPath: (path: string | null) => void
}) {
  const previousPathsRef = useRef<Set<string> | null>(null)
  const { model } = useFileTree({
    density: "compact",
    flattenEmptyDirectories: true,
    gitStatus,
    initialExpansion: "closed",
    initialSelectedPaths: selectedPath ? [selectedPath] : [],
    paths,
    search: false,
    unsafeCSS: TREE_UNSAFE_CSS,
    onSelectionChange: (selectedPaths) => {
      onSelectPath(selectedPaths[0] ? stripTrailingSlash(selectedPaths[0]) : null)
    },
  })

  useEffect(() => {
    const previousPaths = previousPathsRef.current
    const nextPaths = new Set(paths)
    if (!previousPaths) {
      previousPathsRef.current = nextPaths
      model.setGitStatus(gitStatus)
      return
    }
    const removed = [...previousPaths].some((path) => !nextPaths.has(path))
    if (removed) {
      model.resetPaths(paths)
    } else {
      const additions = paths.filter((path) => !previousPaths.has(path))
      if (additions.length > 0) {
        model.batch(additions.map((path) => ({ path, type: "add" })))
      }
    }
    model.setGitStatus(gitStatus)
    previousPathsRef.current = nextPaths
  }, [gitStatus, model, paths])

  useEffect(() => {
    return model.subscribe(() => {
      for (const directoryPath of directoryPaths) {
        if (
          loadedDirectoryPaths.has(directoryPath) ||
          loadingDirectoryPaths.has(directoryPath)
        ) {
          continue
        }
        const item = model.getItem(directoryPath)
        if (item && "isExpanded" in item && item.isExpanded()) {
          onDirectoryExpand(directoryPath)
        }
      }
    })
  }, [
    directoryPaths,
    loadedDirectoryPaths,
    loadingDirectoryPaths,
    model,
    onDirectoryExpand,
  ])

  useEffect(() => {
    if (!selectedPath) return
    model.getItem(selectedPath)?.select()
  }, [model, selectedPath])

  return (
    <FileTree
      aria-label="Repository files"
      className="h-full min-h-0 w-full"
      model={model}
      style={TREE_THEME_STYLE}
    />
  )
}

export function FilesLoadingState({ label }: { label: string }) {
  return (
    <div className="flex h-full flex-col gap-4 px-4 py-5">
      <div className="flex items-center gap-2 text-sm text-muted">
        <Spinner size="sm" aria-label={label} />
        {label}
      </div>
      <TreeSkeleton />
    </div>
  )
}

export function TreeSkeleton() {
  return (
    <div className="flex h-full flex-col gap-2 px-3 py-3" role="status">
      {Array.from({ length: 12 }).map((_, index) => (
        <div
          key={index}
          className="flex h-6 items-center gap-2"
          style={{ paddingLeft: `${(index % 4) * 12}px` }}
        >
          <div className="h-3.5 w-3.5 shrink-0 animate-pulse rounded bg-default" />
          <div
            className="h-3 animate-pulse rounded bg-default"
            style={{ width: `${60 + ((index * 17) % 90)}px` }}
          />
        </div>
      ))}
    </div>
  )
}

export function FilesEmptyState({
  icon,
  title,
  message,
}: {
  icon: string
  title: string
  message: string
}) {
  return (
    <div className="flex h-full items-center justify-center px-6 text-center">
      <div className="flex max-w-sm flex-col items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-border bg-background">
          <Icon icon={icon} className="h-5 w-5 text-muted" />
        </div>
        <div className="text-sm font-medium">{title}</div>
        <p className="text-sm leading-6 text-muted">{message}</p>
      </div>
    </div>
  )
}

export function FilesErrorState({
  message,
  onRetry,
}: {
  message: string
  onRetry: () => void
}) {
  return (
    <div className="flex h-full items-center justify-center px-6 text-center">
      <div className="flex max-w-sm flex-col items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-border bg-background">
          <Icon icon="lucide:circle-alert" className="h-5 w-5 text-muted" />
        </div>
        <div className="text-sm font-medium">Files are not available</div>
        <p className="text-sm leading-6 text-muted">{message}</p>
        <Button size="sm" variant="secondary" onPress={onRetry}>
          Retry
        </Button>
      </div>
    </div>
  )
}

function stripTrailingSlash(path: string) {
  return path.replace(/\/+$/, "")
}
