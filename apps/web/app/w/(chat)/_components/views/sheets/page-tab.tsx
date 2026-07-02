"use client"

import { useRef, useState } from "react"
import {
  AlertDialog,
  Button,
  Dropdown,
  FieldError,
  Input,
  Label,
  Modal,
  Spinner,
  TextField,
  toast,
} from "@heroui/react"
import { Icon } from "@iconify/react"
import { useQueryClient } from "@tanstack/react-query"
import { extractErrorMessage } from "@/lib/api/error"
import {
  deleteSheetPage,
  sheetKeys,
  updateSheetPage,
  type SheetPage,
} from "@/app/w/(chat)/_lib/sheets"

type Mode = "closed" | "menu" | "rename" | "archive"

/**
 * A page tab with a management dropdown (chevron on the active tab, or
 * right-click anywhere on the tab): rename, move left/right via fractional
 * position midpoints, and archive behind a confirm dialog.
 */
export function PageTab({
  sheetId,
  page,
  pages,
  isActive,
  onSelect,
}: {
  sheetId: string
  page: SheetPage
  pages: SheetPage[]
  isActive: boolean
  onSelect: () => void
}) {
  const queryClient = useQueryClient()
  const [mode, setModeState] = useState<Mode>("closed")
  const modeRef = useRef<Mode>("closed")
  const setMode = (next: Mode) => {
    modeRef.current = next
    setModeState(next)
  }
  const [pending, setPending] = useState(false)

  const pageId = page.id ?? ""
  const index = pages.findIndex((entry) => entry.id === page.id)
  const positionOf = (i: number) => {
    const value = pages[i]?.position
    return typeof value === "number" ? value : i
  }

  const refreshStructure = () =>
    queryClient.invalidateQueries({ queryKey: sheetKeys.structure(sheetId) })

  const run = async (work: () => Promise<unknown>, errorMessage: string) => {
    setPending(true)
    try {
      await work()
      setMode("closed")
    } catch (error) {
      toast.danger(extractErrorMessage(error, errorMessage))
    } finally {
      setPending(false)
    }
  }

  const move = (direction: -1 | 1) => {
    const targetIndex = index + direction
    if (index < 0 || targetIndex < 0 || targetIndex >= pages.length) return
    // Fractional midpoints: land between the neighbour and its neighbour.
    let position: number
    if (direction === -1) {
      position =
        targetIndex === 0
          ? positionOf(0) - 1
          : (positionOf(targetIndex - 1) + positionOf(targetIndex)) / 2
    } else {
      position =
        targetIndex === pages.length - 1
          ? positionOf(pages.length - 1) + 1
          : (positionOf(targetIndex) + positionOf(targetIndex + 1)) / 2
    }
    void run(async () => {
      await updateSheetPage(sheetId, pageId, { position })
      await refreshStructure()
    }, "Could not move the page")
  }

  return (
    <>
      <div
        className={`group flex shrink-0 items-center rounded-lg transition-colors ${
          isActive ? "bg-default text-foreground" : "text-muted hover:bg-default"
        }`}
        onContextMenu={(event) => {
          event.preventDefault()
          setMode("menu")
        }}
      >
        <button
          type="button"
          className="flex items-center gap-1.5 py-1 pl-2.5 text-xs"
          onClick={onSelect}
        >
          <span className="max-w-40 truncate">{page.name}</span>
          {typeof page.row_count === "number" ? (
            <span className="text-[10px] text-muted">{page.row_count}</span>
          ) : null}
        </button>
        <Dropdown
          isOpen={mode === "menu"}
          onOpenChange={(open) => {
            if (open) setMode("menu")
            else if (modeRef.current === "menu") setMode("closed")
          }}
        >
          <Dropdown.Trigger
            aria-label={`${page.name ?? "Page"} menu`}
            className={`flex items-center rounded-r-lg py-1 pr-1.5 pl-0.5 text-muted transition-opacity hover:text-foreground ${
              isActive || mode !== "closed"
                ? "opacity-100"
                : "opacity-0 group-hover:opacity-100"
            }`}
          >
            <Icon icon="lucide:chevron-down" className="h-3 w-3" />
          </Dropdown.Trigger>
          <Dropdown.Popover placement="bottom start" className="w-44">
            <Dropdown.Menu
              aria-label={`${page.name ?? "Page"} actions`}
              onAction={(key) => {
                switch (key) {
                  case "rename":
                    setMode("rename")
                    break
                  case "move-left":
                    setMode("closed")
                    move(-1)
                    break
                  case "move-right":
                    setMode("closed")
                    move(1)
                    break
                  case "archive":
                    setMode("archive")
                    break
                }
              }}
            >
              <Dropdown.Item id="rename" textValue="Rename page">
                <Icon icon="lucide:pencil" className="h-3.5 w-3.5 text-muted" />
                Rename
              </Dropdown.Item>
              <Dropdown.Item
                id="move-left"
                textValue="Move left"
                isDisabled={index <= 0}
              >
                <Icon
                  icon="lucide:arrow-left"
                  className="h-3.5 w-3.5 text-muted"
                />
                Move left
              </Dropdown.Item>
              <Dropdown.Item
                id="move-right"
                textValue="Move right"
                isDisabled={index < 0 || index >= pages.length - 1}
              >
                <Icon
                  icon="lucide:arrow-right"
                  className="h-3.5 w-3.5 text-muted"
                />
                Move right
              </Dropdown.Item>
              <Dropdown.Item id="archive" textValue="Archive page" variant="danger">
                <Icon icon="lucide:archive" className="h-3.5 w-3.5" />
                Archive page
              </Dropdown.Item>
            </Dropdown.Menu>
          </Dropdown.Popover>
        </Dropdown>
      </div>

      {mode === "rename" ? (
        <Modal
          isOpen
          onOpenChange={(open) => (!open && !pending ? setMode("closed") : null)}
        >
          <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
            <Modal.Container placement="center" size="sm">
              <Modal.Dialog className="p-8">
                <Modal.CloseTrigger />
                <RenamePageForm
                  page={page}
                  pending={pending}
                  onCancel={() => setMode("closed")}
                  onSubmit={(name) =>
                    run(async () => {
                      await updateSheetPage(sheetId, pageId, { name })
                      await refreshStructure()
                      toast.success("Page renamed")
                    }, "Could not rename the page")
                  }
                />
              </Modal.Dialog>
            </Modal.Container>
          </Modal.Backdrop>
        </Modal>
      ) : null}

      {mode === "archive" ? (
        <AlertDialog>
          <AlertDialog.Backdrop
            isOpen
            onOpenChange={(open) =>
              !open && !pending ? setMode("closed") : null
            }
            className="bg-background/80 backdrop-blur-sm"
          >
            <AlertDialog.Container placement="center" size="sm">
              <AlertDialog.Dialog className="p-8">
                <AlertDialog.Header>
                  <AlertDialog.Icon status="danger">
                    <Icon icon="lucide:archive" className="h-6 w-6" />
                  </AlertDialog.Icon>
                  <div className="flex flex-col gap-1">
                    <AlertDialog.Heading>
                      Archive “{page.name ?? "page"}”
                    </AlertDialog.Heading>
                    <p className="text-sm text-muted">
                      The page and its rows are hidden from the sheet. Agents
                      can no longer read or write to it.
                    </p>
                  </div>
                </AlertDialog.Header>
                <AlertDialog.Footer>
                  <Button
                    variant="tertiary"
                    size="sm"
                    isDisabled={pending}
                    onPress={() => setMode("closed")}
                  >
                    Cancel
                  </Button>
                  <Button
                    variant="danger"
                    size="sm"
                    isDisabled={pending}
                    onPress={() =>
                      run(async () => {
                        await deleteSheetPage(sheetId, pageId)
                        await refreshStructure()
                        toast.success("Page archived")
                      }, "Could not archive the page")
                    }
                  >
                    {pending ? <Spinner color="current" size="sm" /> : null}
                    Archive
                  </Button>
                </AlertDialog.Footer>
              </AlertDialog.Dialog>
            </AlertDialog.Container>
          </AlertDialog.Backdrop>
        </AlertDialog>
      ) : null}
    </>
  )
}

function RenamePageForm({
  page,
  pending,
  onCancel,
  onSubmit,
}: {
  page: SheetPage
  pending: boolean
  onCancel: () => void
  onSubmit: (name: string) => void
}) {
  const [name, setName] = useState(page.name ?? "")
  const invalid = name.trim().length === 0

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault()
        if (!invalid && !pending) onSubmit(name.trim())
      }}
    >
      <Modal.Header>
        <Modal.Heading>Rename page</Modal.Heading>
      </Modal.Header>
      <Modal.Body>
        <TextField
          isInvalid={invalid}
          value={name}
          onChange={setName}
          className="flex flex-col gap-1.5"
        >
          <Label>Page name</Label>
          <Input autoFocus placeholder={page.name ?? "Page name"} />
          <FieldError>Enter a page name.</FieldError>
        </TextField>
      </Modal.Body>
      <Modal.Footer>
        <Button
          type="button"
          variant="tertiary"
          size="sm"
          isDisabled={pending}
          onPress={onCancel}
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
  )
}
