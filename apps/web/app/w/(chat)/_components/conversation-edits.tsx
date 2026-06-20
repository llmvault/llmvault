import { Button } from "@heroui/react"
import { Icon } from "@iconify/react"
import { useState } from "react"
import { Collapse } from "@/app/w/(chat)/_components/conversation-collapse"
import { useWorkspace } from "@/app/w/(chat)/_components/shell"
import type { ConversationBlock } from "@/app/w/(chat)/_lib/static-data"

export function EditsBlock({
  block,
}: {
  block: Extract<ConversationBlock, { type: "edits" }>
}) {
  const [showingAll, setShowingAll] = useState(false)
  const { openView } = useWorkspace()

  return (
    <div className="rounded-2xl border border-border">
      <div className="flex items-center gap-3 px-4 py-3">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-border">
          <Icon icon="lucide:file-diff" className="h-4 w-4 text-muted" />
        </div>
        <div className="flex min-w-0 flex-1 flex-col">
          <span className="text-sm font-medium">
            Edited {block.count} files
          </span>
          <span className="text-sm">
            <span className="text-success">+{block.adds}</span>{" "}
            <span className="text-danger">-{block.dels}</span>
          </span>
        </div>
        <Button variant="ghost" size="sm" className="gap-1.5">
          Undo
          <Icon icon="lucide:rotate-ccw" className="h-3.5 w-3.5" />
        </Button>
        <Button variant="tertiary" size="sm" onPress={() => openView("review")}>
          Review
        </Button>
      </div>
      <div className="border-t border-border px-4 py-2">
        {block.files.map((file) => (
          <FileRow key={file.path} file={file} />
        ))}
        {block.moreFiles?.length ? (
          <>
            <Collapse open={showingAll}>
              {block.moreFiles.map((file) => (
                <FileRow key={file.path} file={file} />
              ))}
            </Collapse>
            <button
              type="button"
              onClick={() => setShowingAll((showing) => !showing)}
              className="flex items-center gap-1.5 py-1.5 text-sm text-muted transition-colors hover:text-foreground"
            >
              <span>
                {showingAll
                  ? "Show fewer files"
                  : `Show ${block.moreFiles.length} more files`}
              </span>
              <Icon
                icon="lucide:chevron-down"
                className={`h-3.5 w-3.5 transition-transform ${showingAll ? "rotate-180" : ""}`}
              />
            </button>
          </>
        ) : null}
      </div>
    </div>
  )
}

function FileRow({
  file,
}: {
  file: { path: string; adds: number; dels: number }
}) {
  return (
    <div className="flex items-center gap-2 py-1.5 text-sm">
      <span className="min-w-0 flex-1 truncate font-mono text-[13px]">
        {file.path}
      </span>
      <span className="text-success">+{file.adds}</span>
      <span className="text-danger">-{file.dels}</span>
    </div>
  )
}
