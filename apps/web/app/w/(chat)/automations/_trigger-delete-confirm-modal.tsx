"use client"

import { ConfirmDialog } from "@/components/confirm-dialog"

export function TriggerDeleteConfirmModal({
  open,
  pending,
  onOpenChange,
  onConfirm,
}: {
  open: boolean
  pending: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}) {
  return (
    <ConfirmDialog
      open={open}
      pending={pending}
      heading="Delete trigger"
      description="This removes the automation and stops future matching Slack reactions from running it."
      confirmLabel="Delete trigger"
      onOpenChange={onOpenChange}
      onConfirm={onConfirm}
    />
  )
}
