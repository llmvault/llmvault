"use client"

import { useRef, useState } from "react"
import {
  AlertDialog,
  Button,
  Description,
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
import { Icon } from "@iconify/react"
import { useQueryClient } from "@tanstack/react-query"
import { extractErrorMessage } from "@/lib/api/error"
import {
  deleteSheetField,
  SHEET_FIELD_TYPES,
  sheetKeys,
  updateSheetField,
  type SheetField,
} from "@/app/w/(chat)/_lib/sheets"
import { fieldTypeIcon } from "./cells"

export interface ColumnMenuTarget {
  field: SheetField
  bounds: { x: number; y: number; width: number; height: number }
}

type Mode = "menu" | "rename" | "type" | "delete"

/**
 * Column header menu: an invisible trigger pinned over the clicked header
 * anchors a HeroUI Dropdown with rename / change type / hide / delete.
 */
export function ColumnHeaderMenu({
  sheetId,
  pageId,
  target,
  onClose,
  onHideField,
}: {
  sheetId: string
  pageId: string
  target: ColumnMenuTarget
  onClose: () => void
  onHideField: (fieldId: string) => void
}) {
  const queryClient = useQueryClient()
  const [mode, setModeState] = useState<Mode>("menu")
  // The dropdown fires onOpenChange(false) right after onAction in the same
  // event; track the mode in a ref so that close does not tear down the
  // follow-up dialog we just opened.
  const modeRef = useRef<Mode>("menu")
  const setMode = (next: Mode) => {
    modeRef.current = next
    setModeState(next)
  }

  const field = target.field
  const fieldId = field.id ?? ""
  const [pending, setPending] = useState(false)

  const refreshStructure = () =>
    queryClient.invalidateQueries({ queryKey: sheetKeys.structure(sheetId) })

  const run = async (work: () => Promise<unknown>, errorMessage: string) => {
    setPending(true)
    try {
      await work()
      onClose()
    } catch (error) {
      toast.danger(extractErrorMessage(error, errorMessage))
    } finally {
      setPending(false)
    }
  }

  return (
    <>
      <Dropdown
        isOpen={mode === "menu"}
        onOpenChange={(open) => {
          if (!open && modeRef.current === "menu") onClose()
        }}
      >
        <Dropdown.Trigger
          aria-label={`${field.name ?? "Column"} menu`}
          className="fixed z-40 opacity-0"
          style={{
            left: target.bounds.x,
            top: target.bounds.y,
            width: target.bounds.width,
            height: target.bounds.height,
          }}
        />
        <Dropdown.Popover placement="bottom start" className="w-52">
          <Dropdown.Menu
            aria-label={`${field.name ?? "Column"} actions`}
            onAction={(key) => {
              switch (key) {
                case "rename":
                  setMode("rename")
                  break
                case "type":
                  setMode("type")
                  break
                case "hide":
                  if (fieldId) onHideField(fieldId)
                  onClose()
                  break
                case "delete":
                  setMode("delete")
                  break
              }
            }}
          >
            <Dropdown.Item id="rename" textValue="Rename column">
              <Icon icon="lucide:pencil" className="h-3.5 w-3.5 text-muted" />
              Rename
            </Dropdown.Item>
            <Dropdown.Item id="type" textValue="Change type">
              <Icon
                icon={fieldTypeIcon(field.type)}
                className="h-3.5 w-3.5 text-muted"
              />
              Change type
            </Dropdown.Item>
            <Dropdown.Item id="hide" textValue="Hide column">
              <Icon icon="lucide:eye-off" className="h-3.5 w-3.5 text-muted" />
              Hide in view
            </Dropdown.Item>
            <Dropdown.Item id="delete" textValue="Delete column" variant="danger">
              <Icon icon="lucide:trash-2" className="h-3.5 w-3.5" />
              Delete column
            </Dropdown.Item>
          </Dropdown.Menu>
        </Dropdown.Popover>
      </Dropdown>

      {mode === "rename" ? (
        <RenameColumnModal
          field={field}
          pending={pending}
          onClose={onClose}
          onSubmit={(name) =>
            run(async () => {
              await updateSheetField(sheetId, pageId, fieldId, { name })
              await refreshStructure()
              toast.success("Column renamed")
            }, "Could not rename the column")
          }
        />
      ) : null}

      {mode === "type" ? (
        <ChangeTypeModal
          field={field}
          pending={pending}
          onClose={onClose}
          onSubmit={(type) =>
            run(async () => {
              await updateSheetField(sheetId, pageId, fieldId, { type })
              await Promise.all([
                refreshStructure(),
                queryClient.invalidateQueries({
                  queryKey: sheetKeys.rowsPrefix(pageId),
                }),
              ])
              toast.success("Column type changed")
            }, "Could not change the column type")
          }
        />
      ) : null}

      {mode === "delete" ? (
        <DeleteColumnDialog
          field={field}
          pending={pending}
          onClose={onClose}
          onConfirm={() =>
            run(async () => {
              await deleteSheetField(sheetId, pageId, fieldId)
              await refreshStructure()
              toast.success("Column deleted")
            }, "Could not delete the column")
          }
        />
      ) : null}
    </>
  )
}

function RenameColumnModal({
  field,
  pending,
  onClose,
  onSubmit,
}: {
  field: SheetField
  pending: boolean
  onClose: () => void
  onSubmit: (name: string) => void
}) {
  const [name, setName] = useState(field.name ?? "")
  const invalid = name.trim().length === 0

  return (
    <Modal isOpen onOpenChange={(open) => (!open && !pending ? onClose() : null)}>
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
                <Modal.Heading>Rename column</Modal.Heading>
              </Modal.Header>
              <Modal.Body>
                <TextField
                  isInvalid={invalid}
                  value={name}
                  onChange={setName}
                  className="flex flex-col gap-1.5"
                >
                  <Label>Column name</Label>
                  <Input autoFocus placeholder={field.name ?? "Column name"} />
                  <FieldError>Enter a column name.</FieldError>
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

function ChangeTypeModal({
  field,
  pending,
  onClose,
  onSubmit,
}: {
  field: SheetField
  pending: boolean
  onClose: () => void
  onSubmit: (type: string) => void
}) {
  const [type, setType] = useState(field.type ?? "text")
  const unchanged = type === (field.type ?? "text")

  return (
    <Modal isOpen onOpenChange={(open) => (!open && !pending ? onClose() : null)}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" size="sm">
          <Modal.Dialog className="p-8">
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Heading>Change column type</Modal.Heading>
            </Modal.Header>
            <Modal.Body className="flex flex-col gap-2">
              <Select
                aria-label="Column type"
                selectedKey={type}
                onSelectionChange={(key) => {
                  if (key !== null) setType(String(key))
                }}
                className="w-full"
              >
                <Select.Trigger className="h-9 w-full justify-between rounded-lg px-3 text-sm">
                  <span className="flex min-w-0 items-center gap-2">
                    <Icon
                      icon={fieldTypeIcon(type)}
                      className="h-3.5 w-3.5 shrink-0 text-muted"
                    />
                    <span className="truncate">{type.replace("_", " ")}</span>
                  </span>
                  <Select.Indicator />
                </Select.Trigger>
                <Select.Popover className="rounded-xl p-1.5">
                  <ListBox>
                    {SHEET_FIELD_TYPES.map((entry) => (
                      <ListBox.Item
                        key={entry}
                        id={entry}
                        textValue={entry.replace("_", " ")}
                      >
                        <span className="flex items-center gap-2">
                          <Icon
                            icon={fieldTypeIcon(entry)}
                            className="h-3.5 w-3.5 text-muted"
                          />
                          {entry.replace("_", " ")}
                        </span>
                      </ListBox.Item>
                    ))}
                  </ListBox>
                </Select.Popover>
              </Select>
              <Description className="text-xs text-muted">
                Existing values that don&apos;t fit the new type show as empty.
                Nothing is deleted — switching back restores them.
              </Description>
            </Modal.Body>
            <Modal.Footer>
              <Button
                variant="tertiary"
                size="sm"
                isDisabled={pending}
                onPress={onClose}
              >
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                isDisabled={unchanged || pending}
                onPress={() => onSubmit(type)}
              >
                {pending ? <Spinner color="current" size="sm" /> : null}
                Change type
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

function DeleteColumnDialog({
  field,
  pending,
  onClose,
  onConfirm,
}: {
  field: SheetField
  pending: boolean
  onClose: () => void
  onConfirm: () => void
}) {
  return (
    <AlertDialog>
      <AlertDialog.Backdrop
        isOpen
        onOpenChange={(open) => (!open && !pending ? onClose() : null)}
        className="bg-background/80 backdrop-blur-sm"
      >
        <AlertDialog.Container placement="center" size="sm">
          <AlertDialog.Dialog className="p-8">
            <AlertDialog.Header>
              <AlertDialog.Icon status="danger">
                <Icon icon="lucide:trash-2" className="h-6 w-6" />
              </AlertDialog.Icon>
              <div className="flex flex-col gap-1">
                <AlertDialog.Heading>
                  Delete “{field.name ?? "column"}”
                </AlertDialog.Heading>
                <p className="text-sm text-muted">
                  The column is archived and disappears from every view. Cell
                  values are kept and can be restored by support.
                </p>
              </div>
            </AlertDialog.Header>
            <AlertDialog.Footer>
              <Button
                variant="tertiary"
                size="sm"
                isDisabled={pending}
                onPress={onClose}
              >
                Cancel
              </Button>
              <Button
                variant="danger"
                size="sm"
                isDisabled={pending}
                onPress={onConfirm}
              >
                {pending ? <Spinner color="current" size="sm" /> : null}
                Delete column
              </Button>
            </AlertDialog.Footer>
          </AlertDialog.Dialog>
        </AlertDialog.Container>
      </AlertDialog.Backdrop>
    </AlertDialog>
  )
}
