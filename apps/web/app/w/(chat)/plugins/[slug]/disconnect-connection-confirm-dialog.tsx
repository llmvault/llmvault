"use client"

import { ConfirmDialog } from "@/components/confirm-dialog"
import { providerLabel } from "@/app/w/(chat)/plugins/[slug]/plugin-detail-helpers"
import type { ConnectionDisconnectTarget } from "@/app/w/(chat)/plugins/[slug]/required-connections-section"

export function DisconnectConnectionConfirmDialog({
  target,
  pending,
  onOpenChange,
  onConfirm,
}: {
  target: ConnectionDisconnectTarget | null
  pending: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}) {
  const provider = target?.provider
    ? providerLabel(target.provider)
    : "connection"
  const detail =
    target?.kind === "database"
      ? "This revokes the stored database credentials and access policy."
      : "This revokes the integration connection and removes provider access."

  return (
    <ConfirmDialog
      open={target !== null}
      pending={pending}
      heading={`Disconnect ${provider}`}
      description={`${detail} Agents will lose access until it is connected again.`}
      confirmLabel="Disconnect"
      icon="unlink"
      onOpenChange={onOpenChange}
      onConfirm={onConfirm}
    />
  )
}
