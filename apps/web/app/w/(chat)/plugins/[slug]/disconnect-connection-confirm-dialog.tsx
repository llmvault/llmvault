"use client"

import { AlertDialog, Button, Spinner } from "@heroui/react"
import { Icon } from "@iconify/react"
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
    <AlertDialog>
      <AlertDialog.Backdrop
        isOpen={target !== null}
        onOpenChange={(next) => {
          if (!pending) onOpenChange(next)
        }}
        className="bg-background/80 backdrop-blur-sm"
      >
        <AlertDialog.Container placement="center" size="sm">
          <AlertDialog.Dialog className="p-8">
            <AlertDialog.Header>
              <AlertDialog.Icon status="danger">
                <Icon icon="lucide:unlink" className="h-6 w-6" />
              </AlertDialog.Icon>
              <div className="flex flex-col gap-1">
                <AlertDialog.Heading>Disconnect {provider}</AlertDialog.Heading>
                <p className="text-sm text-muted">
                  {detail} Agents will lose access until it is connected again.
                </p>
              </div>
            </AlertDialog.Header>
            <AlertDialog.Footer>
              <Button
                variant="tertiary"
                size="sm"
                isDisabled={pending}
                onPress={() => onOpenChange(false)}
              >
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="bg-danger text-danger-foreground hover:bg-danger/90"
                isDisabled={pending}
                onPress={onConfirm}
              >
                {pending ? <Spinner color="current" size="sm" /> : null}
                Disconnect
              </Button>
            </AlertDialog.Footer>
          </AlertDialog.Dialog>
        </AlertDialog.Container>
      </AlertDialog.Backdrop>
    </AlertDialog>
  )
}
