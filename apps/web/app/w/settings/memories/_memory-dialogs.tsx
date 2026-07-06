"use client"

import { useState, type FormEvent } from "react"
import { AlertDialog, Button, Modal, Spinner, TextArea } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import type { Directive, Observation } from "@/lib/api/memory"

export function EditContentModal({
  open,
  heading,
  description,
  initialContent,
  pending,
  onOpenChange,
  onSave,
}: {
  open: boolean
  heading: string
  description: string
  initialContent: string
  pending: boolean
  onOpenChange: (open: boolean) => void
  onSave: (content: string) => void
}) {
  if (!open) return null
  return (
    <EditContentModalContent
      heading={heading}
      description={description}
      initialContent={initialContent}
      pending={pending}
      onOpenChange={onOpenChange}
      onSave={onSave}
    />
  )
}

function EditContentModalContent({
  heading,
  description,
  initialContent,
  pending,
  onOpenChange,
  onSave,
}: {
  heading: string
  description: string
  initialContent: string
  pending: boolean
  onOpenChange: (open: boolean) => void
  onSave: (content: string) => void
}) {
  const [content, setContent] = useState(initialContent)
  const trimmed = content.trim()
  const invalid = trimmed.length === 0
  const unchanged = trimmed === initialContent.trim()

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (invalid || pending) return
    if (unchanged) {
      onOpenChange(false)
      return
    }
    onSave(trimmed)
  }

  return (
    <Modal isOpen onOpenChange={onOpenChange}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" size="sm">
          <Modal.Dialog className="p-8">
            <Modal.CloseTrigger />
            <form onSubmit={submit}>
              <Modal.Header>
                <Modal.Icon className="size-12 bg-default text-foreground">
                  <AppIcon icon="pencil" className="h-6 w-6" />
                </Modal.Icon>
                <div className="flex flex-col gap-1">
                  <Modal.Heading>{heading}</Modal.Heading>
                  <p className="text-sm text-muted">{description}</p>
                </div>
              </Modal.Header>
              <Modal.Body>
                <TextArea
                  autoFocus
                  value={content}
                  disabled={pending}
                  aria-label={heading}
                  rows={4}
                  fullWidth
                  onChange={(event) => setContent(event.target.value)}
                />
              </Modal.Body>
              <Modal.Footer>
                <Button
                  type="button"
                  variant="tertiary"
                  size="sm"
                  isDisabled={pending}
                  onPress={() => onOpenChange(false)}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  variant="primary"
                  size="sm"
                  isDisabled={invalid || unchanged || pending}
                >
                  {pending ? <Spinner color="current" size="sm" /> : null}
                  Save
                </Button>
              </Modal.Footer>
            </form>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

export function DeleteRuleDialog({
  target,
  pending,
  onOpenChange,
  onConfirm,
}: {
  target: Directive | null
  pending: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}) {
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
                <AppIcon icon="trash-2" className="h-6 w-6" />
              </AlertDialog.Icon>
              <div className="flex flex-col gap-1">
                <AlertDialog.Heading>Delete rule</AlertDialog.Heading>
                <p className="text-sm text-muted">
                  Rules can&apos;t be edited. To change this rule, delete it and
                  add a new one.
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
                Delete
              </Button>
            </AlertDialog.Footer>
          </AlertDialog.Dialog>
        </AlertDialog.Container>
      </AlertDialog.Backdrop>
    </AlertDialog>
  )
}

export function DeleteMemoryDialog({
  target,
  pending,
  onOpenChange,
  onConfirm,
}: {
  target: Observation | null
  pending: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}) {
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
                <AppIcon icon="trash-2" className="h-6 w-6" />
              </AlertDialog.Icon>
              <div className="flex flex-col gap-1">
                <AlertDialog.Heading>Delete memory</AlertDialog.Heading>
                <p className="text-sm text-muted">
                  This removes the memory and prevents the agent from
                  re-learning it in this channel.
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
                Delete
              </Button>
            </AlertDialog.Footer>
          </AlertDialog.Dialog>
        </AlertDialog.Container>
      </AlertDialog.Backdrop>
    </AlertDialog>
  )
}
