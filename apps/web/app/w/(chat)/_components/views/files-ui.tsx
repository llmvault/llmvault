"use client"

import { useEffect, useRef, type CSSProperties } from "react"
import type { GitStatusEntry } from "@pierre/trees"
import { FileTree, useFileTree } from "@pierre/trees/react"

export { FilesEmptyState, FilesErrorState } from "./files-empty-state"
export { FilesLoadingState } from "./files-loading-state"
export { TreeSkeleton } from "./files-tree-skeleton"

type TreeCSSProperties = CSSProperties & Record<`--${string}`, string | number>

const TREE_ICONS = {
  colored: true,
  set: "complete",
} as const

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
    icons: TREE_ICONS,
    initialExpansion: "closed",
    initialSelectedPaths: selectedPath ? [selectedPath] : [],
    paths,
    search: false,
    unsafeCSS: TREE_UNSAFE_CSS,
    onSelectionChange: (selectedPaths) => {
      onSelectPath(
        selectedPaths[0] ? stripTrailingSlash(selectedPaths[0]) : null
      )
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

function stripTrailingSlash(path: string) {
  return path.replace(/\/+$/, "")
}
