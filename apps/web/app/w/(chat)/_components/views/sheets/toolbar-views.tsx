"use client"

import { memo, useState } from "react"
import {
  AlertDialog,
  Button,
  Dropdown,
  FieldError,
  Input,
  Label,
  ListBox,
  Modal,
  Select,
  Spinner,
  TextField,
  toast,
} from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import type { SheetView } from "@/app/w/(chat)/_lib/sheets"

export const ViewSwitcher = memo(function ViewSwitcher({
  views,
  activeViewId,
  onSelectView,
  onSaveAsView,
  onRenameView,
  onDeleteView,
}: {
  views: SheetView[]
  activeViewId: string | null
  onSelectView: (viewId: string | null) => void
  onSaveAsView: (name: string) => Promise<void>
  onRenameView: (name: string) => Promise<void>
  onDeleteView: () => Promise<void>
}) {
  const [dialog, setDialog] = useState<"save" | "rename" | "delete" | null>(
    null
  )
  const [pending, setPending] = useState(false)
  const activeView = views.find((view) => view.id === activeViewId) ?? null

  const run = async (work: () => Promise<void>, errorMessage: string) => {
    setPending(true)
    try {
      await work()
      setDialog(null)
    } catch (error) {
      toast.danger(extractErrorMessage(error, errorMessage))
    } finally {
      setPending(false)
    }
  }

  return (
    <>
      <div className="flex items-center">
        {views.length > 0 ? (
          <Select
            aria-label="View"
            selectedKey={activeViewId}
            onSelectionChange={(key) => {
              if (key !== null) onSelectView(String(key))
            }}
            className="min-w-0"
          >
            <Select.Trigger className="flex h-8 max-w-44 items-center gap-1.5 rounded-l-lg rounded-r-none border border-r-0 border-border px-2 text-xs text-muted transition-colors hover:bg-default">
              <AppIcon icon="layout-grid" className="h-3.5 w-3.5" />
              <span className="truncate text-foreground">
                {activeView?.name ?? "View"}
              </span>
              <Select.Indicator />
            </Select.Trigger>
            <Select.Popover className="min-w-48 rounded-xl p-1">
              <ListBox>
                {views.map((view) => (
                  <ListBox.Item
                    key={view.id}
                    id={view.id ?? ""}
                    textValue={view.name ?? ""}
                  >
                    <span className="flex items-center gap-2 text-xs">
                      <AppIcon
                        icon="layout-grid"
                        className="h-3.5 w-3.5 text-muted"
                      />
                      {view.name}
                    </span>
                  </ListBox.Item>
                ))}
              </ListBox>
            </Select.Popover>
          </Select>
        ) : null}
        <Dropdown>
          <Dropdown.Trigger
            aria-label="View actions"
            className={`flex h-8 items-center gap-1 border border-border px-1.5 text-xs text-muted transition-colors hover:bg-default ${
              views.length > 0 ? "rounded-r-lg" : "rounded-lg px-2"
            }`}
          >
            {views.length > 0 ? (
              <AppIcon icon="ellipsis" className="h-3.5 w-3.5" />
            ) : (
              <>
                <AppIcon icon="layout-grid" className="h-3.5 w-3.5" />
                Views
              </>
            )}
          </Dropdown.Trigger>
          <Dropdown.Popover placement="bottom start" className="w-52">
            <Dropdown.Menu
              aria-label="View actions"
              onAction={(key) => {
                if (key === "save") setDialog("save")
                if (key === "rename") setDialog("rename")
                if (key === "delete") setDialog("delete")
              }}
            >
              <Dropdown.Item id="save" textValue="Save as view">
                <AppIcon icon="plus" className="h-3.5 w-3.5 text-muted" />
                Save as new view
              </Dropdown.Item>
              <Dropdown.Item
                id="rename"
                textValue="Rename view"
                isDisabled={!activeView}
              >
                <AppIcon icon="pencil" className="h-3.5 w-3.5 text-muted" />
                Rename view
              </Dropdown.Item>
              <Dropdown.Item
                id="delete"
                textValue="Delete view"
                variant="danger"
                isDisabled={!activeView}
              >
                <AppIcon icon="trash-2" className="h-3.5 w-3.5" />
                Delete view
              </Dropdown.Item>
            </Dropdown.Menu>
          </Dropdown.Popover>
        </Dropdown>
      </div>

      {dialog === "save" || dialog === "rename" ? (
        <ViewNameModal
          title={dialog === "save" ? "Save as view" : "Rename view"}
          submitLabel={dialog === "save" ? "Save view" : "Save"}
          initialName={
            dialog === "rename" ? (activeView?.name ?? "") : ""
          }
          pending={pending}
          onClose={() => (!pending ? setDialog(null) : null)}
          onSubmit={(name) =>
            void run(
              () =>
                dialog === "save" ? onSaveAsView(name) : onRenameView(name),
              dialog === "save"
                ? "Could not save the view"
                : "Could not rename the view"
            )
          }
        />
      ) : null}

      {dialog === "delete" ? (
        <AlertDialog>
          <AlertDialog.Backdrop
            isOpen
            onOpenChange={(open) => (!open && !pending ? setDialog(null) : null)}
            className="bg-background/80 backdrop-blur-sm"
          >
            <AlertDialog.Container placement="center" size="sm">
              <AlertDialog.Dialog className="p-8">
                <AlertDialog.Header>
                  <AlertDialog.Icon status="danger">
                    <AppIcon icon="trash-2" className="h-6 w-6" />
                  </AlertDialog.Icon>
                  <div className="flex flex-col gap-1">
                    <AlertDialog.Heading>
                      Delete “{activeView?.name ?? "view"}”
                    </AlertDialog.Heading>
                    <p className="text-sm text-muted">
                      Only the saved view configuration is removed — rows and
                      columns are untouched.
                    </p>
                  </div>
                </AlertDialog.Header>
                <AlertDialog.Footer>
                  <Button
                    variant="tertiary"
                    size="sm"
                    isDisabled={pending}
                    onPress={() => setDialog(null)}
                  >
                    Cancel
                  </Button>
                  <Button
                    variant="danger"
                    size="sm"
                    isDisabled={pending}
                    onPress={() =>
                      void run(onDeleteView, "Could not delete the view")
                    }
                  >
                    {pending ? <Spinner color="current" size="sm" /> : null}
                    Delete view
                  </Button>
                </AlertDialog.Footer>
              </AlertDialog.Dialog>
            </AlertDialog.Container>
          </AlertDialog.Backdrop>
        </AlertDialog>
      ) : null}
    </>
  )
})

function ViewNameModal({
  title,
  submitLabel,
  initialName,
  pending,
  onClose,
  onSubmit,
}: {
  title: string
  submitLabel: string
  initialName: string
  pending: boolean
  onClose: () => void
  onSubmit: (name: string) => void
}) {
  const [name, setName] = useState(initialName)
  const invalid = name.trim().length === 0

  return (
    <Modal isOpen onOpenChange={(open) => (!open ? onClose() : null)}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" size="sm">
          <Modal.Dialog className="p-8">
            <Modal.CloseTrigger />
            <form
              onSubmit={(event) => {
                event.preventDefault()
                if (!invalid && !pending) onSubmit(name.trim())
              }}
            >
              <Modal.Header>
                <Modal.Heading>{title}</Modal.Heading>
              </Modal.Header>
              <Modal.Body>
                <TextField
                  isInvalid={invalid}
                  value={name}
                  onChange={setName}
                  className="flex flex-col gap-1.5"
                >
                  <Label>View name</Label>
                  <Input autoFocus placeholder="e.g. Active deals" />
                  <FieldError>Enter a view name.</FieldError>
                </TextField>
              </Modal.Body>
              <Modal.Footer>
                <Button
                  type="button"
                  variant="tertiary"
                  size="sm"
                  isDisabled={pending}
                  onPress={onClose}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  variant="primary"
                  size="sm"
                  isDisabled={invalid || pending}
                >
                  {pending ? <Spinner color="current" size="sm" /> : null}
                  {submitLabel}
                </Button>
              </Modal.Footer>
            </form>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
