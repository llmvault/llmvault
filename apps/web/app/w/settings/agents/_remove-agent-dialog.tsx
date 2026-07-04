"use client"

import { AlertDialog, Button, Spinner } from "@heroui/react"
import { AppIcon } from "@/components/icon"

// Danger-styled confirmation used before removing an agent — either uninstalling
// a catalog agent or deleting an org-created one. The parent owns the mutation
// and passes the copy so a single component covers both flows.
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
                <AppIcon icon="trash-2" className="h-6 w-6" />
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
                {confirmLabel}
              </Button>
            </AlertDialog.Footer>
          </AlertDialog.Dialog>
        </AlertDialog.Container>
      </AlertDialog.Backdrop>
    </AlertDialog>
  )
}
