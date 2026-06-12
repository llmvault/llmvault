"use client"

import { useState } from "react"
import { Icon } from "@iconify/react"
import {
  fileTree,
  fileContents,
  fileContentsByPath,
  type FileTreeNode,
} from "../../_lib/static-data"

const FALLBACK_LINES = [
  "// Static design exploration — select a file under",
  "// internal/handler to see sample contents.",
]

export function FilesView() {
  const [openFolders, setOpenFolders] = useState<Set<string>>(
    () => new Set(["internal", "internal/handler"])
  )
  const [activePath, setActivePath] = useState(fileContents.path)

  const toggleFolder = (path: string) => {
    setOpenFolders((current) => {
      const next = new Set(current)
      if (next.has(path)) {
        next.delete(path)
      } else {
        next.add(path)
      }
      return next
    })
  }

  const lines =
    activePath === fileContents.path
      ? fileContents.lines
      : (fileContentsByPath[activePath] ?? FALLBACK_LINES)

  return (
    <div className="flex h-full min-w-0">
      <div className="w-56 shrink-0 overflow-y-auto border-r border-border bg-surface py-2">
        {fileTree.map((node) => (
          <TreeNode
            key={node.name}
            node={node}
            parentPath=""
            depth={0}
            openFolders={openFolders}
            activePath={activePath}
            onToggleFolder={toggleFolder}
            onSelectFile={setActivePath}
          />
        ))}
      </div>

      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex shrink-0 items-center gap-2 border-b border-border px-4 py-2">
          <Icon icon="lucide:file-code" className="h-4 w-4 shrink-0 text-muted" />
          <span className="truncate font-mono text-xs text-muted">
            {activePath}
          </span>
        </div>
        <div className="min-h-0 flex-1 overflow-auto py-3 font-mono text-[13px] leading-6">
          {lines.map((line, index) => (
            <div key={index} className="flex">
              <span className="w-12 shrink-0 select-none pr-4 text-right text-muted/50">
                {index + 1}
              </span>
              <pre className="flex-1 whitespace-pre pr-4">{line || " "}</pre>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

function TreeNode({
  node,
  parentPath,
  depth,
  openFolders,
  activePath,
  onToggleFolder,
  onSelectFile,
}: {
  node: FileTreeNode
  parentPath: string
  depth: number
  openFolders: Set<string>
  activePath: string
  onToggleFolder: (path: string) => void
  onSelectFile: (path: string) => void
}) {
  const path = parentPath ? `${parentPath}/${node.name}` : node.name
  const isFolder = Boolean(node.children)
  const isOpen = openFolders.has(path)

  return (
    <div>
      <button
        type="button"
        onClick={() => (isFolder ? onToggleFolder(path) : onSelectFile(path))}
        className={`flex w-full items-center gap-1.5 py-1 pr-2 text-left text-sm transition-colors hover:bg-default ${
          path === activePath ? "bg-default" : ""
        }`}
        style={{ paddingLeft: `${depth * 14 + 10}px` }}
      >
        {isFolder ? (
          <Icon
            icon={isOpen ? "lucide:chevron-down" : "lucide:chevron-right"}
            className="h-3.5 w-3.5 shrink-0 text-muted"
          />
        ) : (
          <span className="w-3.5 shrink-0" />
        )}
        <Icon
          icon={isFolder ? "lucide:folder" : "lucide:file-code"}
          className="h-4 w-4 shrink-0 text-muted"
        />
        <span className="min-w-0 flex-1 truncate">{node.name}</span>
      </button>
      {isFolder && isOpen
        ? node.children?.map((child) => (
            <TreeNode
              key={child.name}
              node={child}
              parentPath={path}
              depth={depth + 1}
              openFolders={openFolders}
              activePath={activePath}
              onToggleFolder={onToggleFolder}
              onSelectFile={onSelectFile}
            />
          ))
        : null}
    </div>
  )
}
