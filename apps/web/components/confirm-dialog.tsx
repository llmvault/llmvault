"use client"

import { AlertDialog, Button, Spinner } from "@heroui/react"
import { AppIcon } from "@/components/icon"

/**
 * Shared danger-styled confirmation dialog. The parent owns the mutation and
 * passes the copy, so one component covers every "are you sure?" flow (delete
 * channel, remove agent, disconnect connection, …).
 */
export function ConfirmDialog({
  open,
  pending,
  heading,
  description,
  confirmLabel,
  cancelLabel = "Cancel",
  icon = "trash-2",
  onOpenChange,
  onConfirm,
}: {
  open: boolean
  pending: boolean
  heading: string
  description: string
  confirmLabel: string
  cancelLabel?: string
  icon?: string
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}) {
  return (
    <AlertDialog>
      <AlertDialog.Backdrop
        isOpen={open}
        onOpenChange={(next) => {
          if (!pending) onOpenChange(next)
        }}
        className="bg-background/80 backdrop-blur-sm"
      >
        <AlertDialog.Container placement="center" size="sm">
          <AlertDialog.Dialog className="p-8">
            <AlertDialog.Header>
              <AlertDialog.Icon status="danger">
                <AppIcon icon={icon} className="h-6 w-6" />
              </AlertDialog.Icon>
              <div className="flex flex-col gap-1">
                <AlertDialog.Heading>{heading}</AlertDialog.Heading>
                <p className="text-sm text-muted">{description}</p>
              </div>
            </AlertDialog.Header>
            <AlertDialog.Footer>
              <Button
                variant="tertiary"
                size="sm"
                isDisabled={pending}
                onPress={() => onOpenChange(false)}
              >
                {cancelLabel}
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="bg-danger text-danger-foreground hover:bg-danger/90"
                isDisabled={pending}
                onPress={onConfirm}
              >
                {pending ? <Spinner color="current" size="sm" /> : null}
                {confirmLabel}
              </Button>
            </AlertDialog.Footer>
          </AlertDialog.Dialog>
        </AlertDialog.Container>
      </AlertDialog.Backdrop>
    </AlertDialog>
  )
}
