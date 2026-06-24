"use client"

import { Button, Modal } from "@heroui/react"
import { Icon } from "@iconify/react"

export function MicrophonePermissionModal({
  open,
  onOpenChange,
  onConfirm,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}) {
  return (
    <Modal isOpen={open} onOpenChange={onOpenChange}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" size="sm">
          <Modal.Dialog className="p-8">
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Icon className="bg-default size-12 text-foreground">
                <Icon icon="lucide:mic" className="h-6 w-6" />
              </Modal.Icon>
              <div className="flex flex-col gap-1">
                <Modal.Heading>Enable microphone</Modal.Heading>
                <p className="text-sm text-muted">
                  Hivy needs microphone access to record your voice. Your
                  browser may ask you to allow access after this step.
                </p>
              </div>
            </Modal.Header>
            <Modal.Footer>
              <Button
                variant="tertiary"
                size="sm"
                onPress={() => onOpenChange(false)}
              >
                Cancel
              </Button>
              <Button variant="primary" size="sm" onPress={onConfirm}>
                Enable microphone
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
