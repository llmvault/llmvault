"use client"

import { useState } from "react"
import { Popover } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import type { RuntimeRepoInfo } from "@/app/w/(chat)/_lib/runtime-repos"

export interface FilesRepoSelectorProps {
  repos: RuntimeRepoInfo[]
  selectedRepo: RuntimeRepoInfo | null
  onSelect: (repoId: string) => void
}

export function FilesRepoSelector({
  repos,
  selectedRepo,
  onSelect,
}: FilesRepoSelectorProps) {
  const [open, setOpen] = useState(false)
  if (!selectedRepo) return null
  if (repos.length === 1) {
    return (
      <span className="bg-surface-secondary max-w-28 min-w-0 truncate rounded-md px-1.5 py-0.5 text-xs text-muted">
        {selectedRepo.name}
      </span>
    )
  }
  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger className="hover:bg-surface-secondary flex max-w-32 min-w-0 items-center gap-1 rounded-md px-1.5 py-0.5 text-xs text-muted transition-colors">
        <span className="truncate">{selectedRepo.name}</span>
        <AppIcon icon="chevron-down" className="h-3 w-3 shrink-0" />
      </Popover.Trigger>
      <Popover.Content className="bg-surface w-64 rounded-2xl border border-border p-1.5">
        <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
          {repos.map((repo) => (
            <button
              key={repo.id}
              type="button"
              className={`hover:bg-surface-secondary flex min-w-0 items-center gap-2 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors ${
                repo.id === selectedRepo.id ? "text-foreground" : "text-muted"
              }`}
              onClick={() => {
                onSelect(repo.id)
                setOpen(false)
              }}
            >
              <AppIcon icon="git-branch" className="h-4 w-4 shrink-0" />
              <span className="min-w-0 flex-1 truncate">{repo.name}</span>
            </button>
          ))}
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
}
