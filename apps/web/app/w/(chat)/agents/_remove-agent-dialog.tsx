"use client"

import { ConfirmDialog } from "@/components/confirm-dialog"

// Danger-styled confirmation used before removing an agent — either uninstalling
// a catalog agent or deleting an org-created one. The parent owns the mutation
// and passes the copy so a single component covers both flows. Delegates to the
// shared ConfirmDialog so there is one confirm-dialog implementation.
export function RemoveAgentDialog({
  open,
  pending,
  heading,
  description,
  confirmLabel,
  onOpenChange,
  onConfirm,
}: {
  open: boolean
  pending: boolean
  heading: string
  description: string
  confirmLabel: string
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}) {
  return (
    <ConfirmDialog
      open={open}
      pending={pending}
      heading={heading}
      description={description}
      confirmLabel={confirmLabel}
      onOpenChange={onOpenChange}
      onConfirm={onConfirm}
    />
  )
}
