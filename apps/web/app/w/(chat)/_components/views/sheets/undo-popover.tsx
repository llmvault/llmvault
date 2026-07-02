"use client"

import { useState } from "react"
import { Button, Popover, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { extractErrorMessage } from "@/lib/api/error"
import {
  fetchOperations,
  revertOperation,
  sheetKeys,
  type SheetOperation,
} from "@/app/w/(chat)/_lib/sheets"

function operationLabel(type: string | undefined): string {
  switch (type) {
    case "rows_insert":
      return "Added rows"
    case "rows_update":
      return "Updated rows"
    case "rows_delete":
      return "Deleted rows"
    case "csv_import":
      return "CSV import"
    case "field_change":
      return "Field change"
    default:
      return type ?? "Operation"
  }
}

function operationIcon(type: string | undefined): string {
  switch (type) {
    case "rows_insert":
      return "plus"
    case "rows_update":
      return "pencil"
    case "rows_delete":
      return "trash-2"
    case "csv_import":
      return "file-up"
    case "field_change":
      return "columns-3"
    default:
      return "history"
  }
}

function actorLabel(operation: SheetOperation): string {
  if (operation.actor_agent_id) return "Agent"
  if (operation.actor_user_id) return "User"
  return "System"
}

function relativeTime(iso: string | undefined): string {
  if (!iso) return ""
  const then = new Date(iso).getTime()
  if (!Number.isFinite(then)) return ""
  const seconds = Math.max(0, Math.round((Date.now() - then) / 1000))
  if (seconds < 60) return "just now"
  const minutes = Math.round(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.round(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.round(hours / 24)}d ago`
}

export function UndoPopover({
  sheetId,
  pageId,
}: {
  sheetId: string
  pageId: string
}) {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [revertingId, setRevertingId] = useState<string | null>(null)

  const operationsQuery = useQuery({
    enabled: open,
    queryKey: sheetKeys.operations(pageId),
    queryFn: ({ signal }) => fetchOperations(sheetId, pageId, signal),
  })
  const operations = operationsQuery.data?.operations ?? []

  const revert = async (operation: SheetOperation) => {
    if (!operation.id) return
    setRevertingId(operation.id)
    try {
      await revertOperation(sheetId, pageId, operation.id)
      toast.success("Operation reverted")
      void queryClient.invalidateQueries({
        queryKey: sheetKeys.rowsPrefix(pageId),
      })
      void queryClient.invalidateQueries({
        queryKey: sheetKeys.operations(pageId),
      })
      void queryClient.invalidateQueries({
        queryKey: sheetKeys.structure(sheetId),
      })
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not revert"))
    } finally {
      setRevertingId(null)
    }
  }

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger className="flex h-8 items-center gap-1.5 rounded-lg border border-border px-2 text-xs text-muted transition-colors hover:bg-default">
        <AppIcon icon="undo-2" className="h-3.5 w-3.5" />
        Undo
      </Popover.Trigger>
      <Popover.Content className="w-80 rounded-2xl border border-border p-2">
        <Popover.Dialog className="flex max-h-80 w-full flex-col gap-0.5 overflow-y-auto p-0">
          {operationsQuery.isPending ? (
            <div className="flex justify-center py-4">
              <Spinner size="sm" />
            </div>
          ) : operationsQuery.isError ? (
            <p className="px-2 py-3 text-xs text-muted">
              {extractErrorMessage(
                operationsQuery.error,
                "Could not load operations."
              )}
            </p>
          ) : operations.length === 0 ? (
            <p className="px-2 py-3 text-xs text-muted">
              No recent operations on this page.
            </p>
          ) : (
            operations.map((operation) => (
              <div
                key={operation.id}
                className="flex items-center gap-2 rounded-lg px-2 py-1.5 hover:bg-default"
              >
                <AppIcon
                  icon={operationIcon(operation.type)}
                  className="h-3.5 w-3.5 shrink-0 text-muted"
                />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-xs text-foreground">
                    {operationLabel(operation.type)}
                    {operation.row_count
                      ? ` · ${operation.row_count} row${
                          operation.row_count === 1 ? "" : "s"
                        }`
                      : ""}
                  </p>
                  <p className="truncate text-[10px] text-muted">
                    {actorLabel(operation)} · {relativeTime(operation.created_at)}
                    {operation.reverted_at ? " · reverted" : ""}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  isDisabled={
                    Boolean(operation.reverted_at) ||
                    revertingId === operation.id
                  }
                  onPress={() => void revert(operation)}
                >
                  {revertingId === operation.id ? (
                    <Spinner size="sm" />
                  ) : (
                    "Revert"
                  )}
                </Button>
              </div>
            ))
          )}
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
}
