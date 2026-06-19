"use client"

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ComponentType,
} from "react"
import { Button, Popover } from "@heroui/react"
import { Icon } from "@iconify/react"
import { useDropzone } from "react-dropzone"
import { cn } from "@/lib/utils"
import {
  useOrgDriveFileUploads,
  type OrgDriveUploadItem,
} from "@/hooks/use-org-drive-file-uploads"
import { modelById, type Agent } from "@/app/w/(chat)/_lib/agents"
import {
  attachmentMetadataFromDescription,
  describeDriveImage,
  type ImageAttachmentMetadata,
} from "@/app/w/(chat)/_lib/image-attachments"
import {
  AttachmentPreviewTray,
  type AttachmentDescriptionState,
  type ComposerImageAttachment,
} from "./composer-attachments"
import {
  codeLineCommentPayloads,
  formatCodeLineCommentLine,
  useCodeLineCommentActions,
  useCodeLineComments,
  type CodeLineComment,
} from "@/app/w/(chat)/_components/line-comments"
import type { CodeLineCommentPayload } from "@/app/w/(chat)/_lib/code-line-comments"

const ACCESS_MODES = [
  {
    id: "full",
    label: "Full access",
    icon: "lucide:octagon-alert",
    description: "Edit files, run commands, and use the network",
    warning: true,
  },
  {
    id: "edits",
    label: "Approve edits",
    icon: "lucide:file-check-2",
    description: "Ask before changing files or running commands",
    warning: false,
  },
  {
    id: "read",
    label: "Read only",
    icon: "lucide:eye",
    description: "Explore the workspace without making changes",
    warning: false,
  },
]

const EFFORTS = ["Low", "Medium", "High"]

type DisplayModel = {
  id: string
  label: string
  provider: string
  Icon?: ComponentType<{ className?: string }>
}

function displayModel(id: string): DisplayModel {
  try {
    return modelById(id)
  } catch {
    return {
      id,
      label: id,
      provider: "Agent model",
    }
  }
}

function ModelIcon({
  model,
  className,
}: {
  model: DisplayModel
  className: string
}) {
  const IconComponent = model.Icon
  if (IconComponent) {
    return <IconComponent className={className} />
  }
  return <Icon icon="lucide:brain" className={`${className} text-muted`} />
}

function ComposerLineComments({
  comments,
  onClear,
  onRemove,
}: {
  comments: CodeLineComment[]
  onClear: () => void
  onRemove: (id: string) => void
}) {
  return (
    <div className="group relative flex items-start px-1">
      <button
        type="button"
        aria-label={`${comments.length} ${comments.length === 1 ? "code comment" : "code comments"}`}
        aria-haspopup="dialog"
        className="hover:bg-default bg-surface-secondary focus-visible:ring-offset-surface flex h-8 items-center gap-2 rounded-2xl border border-border px-3 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:outline-none"
        onClick={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
      >
        <Icon icon="lucide:message-square" className="h-4 w-4 text-muted" />
        {comments.length} {comments.length === 1 ? "comment" : "comments"}
      </button>
      <div
        className="absolute bottom-full left-1 z-50 hidden w-[min(24rem,calc(100vw-2rem))] pb-2 group-focus-within:block group-hover:block"
        onClick={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
        aria-label="Code comments"
        role="dialog"
      >
        <div className="bg-surface flex max-h-80 w-full flex-col gap-1 overflow-y-auto rounded-2xl border border-border p-2 shadow-xl">
          <div className="flex items-center justify-between px-2 py-1">
            <span className="text-xs font-medium text-muted">
              Comments sent with next message
            </span>
            <button
              type="button"
              className="text-xs text-muted transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
              onClick={(event) => {
                event.stopPropagation()
                onClear()
              }}
            >
              Clear
            </button>
          </div>
          {comments.map((comment) => (
            <div
              key={comment.id}
              className="hover:bg-default group/comment rounded-xl px-2 py-2 transition-colors"
            >
              <div className="flex min-w-0 items-start gap-2">
                <div className="min-w-0 flex-1">
                  <div className="truncate font-mono text-xs text-muted">
                    {comment.displayPath}
                  </div>
                  <div className="mt-0.5 text-xs font-medium">
                    Line {formatCodeLineCommentLine(comment)}
                  </div>
                </div>
                <button
                  type="button"
                  aria-label="Remove comment"
                  className="hover:bg-surface-tertiary rounded-md p-1 text-muted opacity-0 transition-opacity group-hover/comment:opacity-100 focus:opacity-100 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                  onClick={(event) => {
                    event.stopPropagation()
                    onRemove(comment.id)
                  }}
                >
                  <Icon icon="lucide:x" className="h-3.5 w-3.5" />
                </button>
              </div>
              <p className="mt-2 line-clamp-4 text-sm leading-5 whitespace-pre-wrap">
                {comment.body}
              </p>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

export function Composer({
  agent,
  agentId,
  modelId,
  onModelChange,
  onSend,
  placeholder = "Ask for follow-up changes",
  isStreaming = false,
  onStop,
}: {
  agent: Agent
  agentId: string
  modelId: string
  onModelChange: (modelId: string) => void
  onSend: (
    text: string,
    attachments: ImageAttachmentMetadata[],
    codeLineComments: CodeLineCommentPayload[]
  ) => boolean | void | Promise<boolean | void>
  placeholder?: string
  isStreaming?: boolean
  onStop?: () => void
}) {
  const [value, setValue] = useState("")
  const [accessMode, setAccessMode] = useState(ACCESS_MODES[0])
  const [effort, setEffort] = useState("High")
  const [accessOpen, setAccessOpen] = useState(false)
  const [modelOpen, setModelOpen] = useState(false)
  const [recording, setRecording] = useState(false)
  const lineComments = useCodeLineComments()
  const lineCommentActions = useCodeLineCommentActions()
  const [attachmentDescriptions, setAttachmentDescriptions] = useState<
    Record<string, AttachmentDescriptionState>
  >({})
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)
  const describedUploadsRef = useRef<Set<string>>(new Set())
  const { uploads, addFiles, retryUpload, removeUpload, clearUploads } =
    useOrgDriveFileUploads({ agentId })

  const selectedModel = displayModel(modelId)
  const attachments = useMemo(
    () =>
      uploads.map((upload): ComposerImageAttachment => {
        if (upload.status === "uploading") {
          return { upload, status: "uploading" }
        }
        if (upload.status === "error") {
          return {
            upload,
            status: "error",
            error: upload.error || "Image upload failed",
          }
        }
        if (!upload.asset?.content_type.startsWith("image/")) {
          return {
            upload,
            status: "error",
            error: "Uploaded file is not an image",
          }
        }

        const description = attachmentDescriptions[upload.id]
        if (description?.status === "ready") {
          return {
            upload,
            status: "ready",
            metadata: description.metadata,
          }
        }
        if (description?.status === "error") {
          return {
            upload,
            status: "error",
            error: description.error,
          }
        }
        return { upload, status: "describing" }
      }),
    [attachmentDescriptions, uploads]
  )
  const hasPendingAttachment = attachments.some(
    (attachment) =>
      attachment.status === "uploading" || attachment.status === "describing"
  )
  const hasFailedAttachment = attachments.some(
    (attachment) => attachment.status === "error"
  )
  const readyAttachments = attachments
    .map((attachment) => attachment.metadata)
    .filter((attachment): attachment is ImageAttachmentMetadata =>
      Boolean(attachment)
    )
  const canSend =
    !hasPendingAttachment &&
    !hasFailedAttachment &&
    (value.trim().length > 0 ||
      readyAttachments.length > 0 ||
      lineComments.length > 0)

  const describeUpload = useCallback(async (upload: OrgDriveUploadItem) => {
    if (!upload.asset) return
    setAttachmentDescriptions((current) => ({
      ...current,
      [upload.id]: { status: "describing" },
    }))
    try {
      const description = await describeDriveImage(upload.asset.id)
      const metadata = attachmentMetadataFromDescription(
        upload.asset,
        description
      )
      setAttachmentDescriptions((current) => ({
        ...current,
        [upload.id]: { status: "ready", metadata },
      }))
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Image processing failed"
      setAttachmentDescriptions((current) => ({
        ...current,
        [upload.id]: { status: "error", error: message },
      }))
    }
  }, [])

  useEffect(() => {
    for (const upload of uploads) {
      if (upload.status !== "uploaded" || !upload.asset) continue
      if (describedUploadsRef.current.has(upload.id)) continue
      describedUploadsRef.current.add(upload.id)
      if (!upload.asset.content_type.startsWith("image/")) {
        window.queueMicrotask(() => {
          setAttachmentDescriptions((current) => ({
            ...current,
            [upload.id]: {
              status: "error",
              error: "Uploaded file is not an image",
            },
          }))
        })
        continue
      }
      window.queueMicrotask(() => {
        void describeUpload(upload)
      })
    }
  }, [describeUpload, uploads])

  const retryAttachment = (attachment: ComposerImageAttachment) => {
    const id = attachment.upload.id
    setAttachmentDescriptions((current) => {
      const next = { ...current }
      delete next[id]
      return next
    })
    describedUploadsRef.current.delete(id)
    if (attachment.upload.status === "error" || !attachment.upload.asset) {
      retryUpload(id)
      return
    }
    describedUploadsRef.current.add(id)
    void describeUpload(attachment.upload)
  }

  const removeAttachment = (id: string) => {
    removeUpload(id)
    describedUploadsRef.current.delete(id)
    setAttachmentDescriptions((current) => {
      const next = { ...current }
      delete next[id]
      return next
    })
  }

  const resetAttachments = () => {
    clearUploads()
    describedUploadsRef.current.clear()
    setAttachmentDescriptions({})
  }

  const onDropAccepted = useCallback(
    (files: File[]) => {
      addFiles(files)
    },
    [addFiles]
  )

  const { getRootProps, getInputProps, open, isDragActive, isDragReject } =
    useDropzone({
      accept: { "image/*": [] },
      multiple: true,
      noClick: true,
      noKeyboard: true,
      onDropAccepted,
    })

  const submit = async () => {
    if (!canSend || isStreaming) {
      return
    }

    const promptText = value.trim()
    const sendingAttachments = readyAttachments
    const sendingLineComments = lineComments
    const sendingLineCommentIds = sendingLineComments.map(
      (comment) => comment.id
    )
    const sendingCodeLineComments = codeLineCommentPayloads(sendingLineComments)
    setValue("")
    try {
      const sent = await onSend(
        promptText,
        sendingAttachments,
        sendingCodeLineComments
      )
      if (sent === false) {
        setValue((current) => (current === "" ? promptText : current))
        return
      }
      resetAttachments()
      lineCommentActions.removeComments(sendingLineCommentIds)
      const node = textareaRef.current
      if (node) {
        node.style.height = "auto"
      }
    } catch {
      setValue((current) => (current === "" ? promptText : current))
    }
  }

  return (
    <div className="mx-auto w-full max-w-3xl px-4 pb-4">
      <div
        {...getRootProps({
          className: cn(
            "bg-surface flex flex-col gap-2 rounded-3xl border px-3 pt-3 pb-2 shadow-sm transition-colors",
            isDragActive && !isDragReject
              ? "border-primary bg-primary/5"
              : "border-border",
            isDragReject && "border-danger bg-danger/5"
          ),
        })}
      >
        <input {...getInputProps({ "aria-label": "Attach images" })} />
        {attachments.length ? (
          <AttachmentPreviewTray
            attachments={attachments}
            onRetry={retryAttachment}
            onRemove={removeAttachment}
          />
        ) : null}
        {lineComments.length ? (
          <ComposerLineComments
            comments={lineComments}
            onClear={() =>
              lineCommentActions.removeComments(
                lineComments.map((comment) => comment.id)
              )
            }
            onRemove={lineCommentActions.removeComment}
          />
        ) : null}
        <textarea
          ref={textareaRef}
          rows={1}
          value={value}
          placeholder={placeholder}
          onChange={(event) => {
            setValue(event.target.value)
            event.target.style.height = "auto"
            event.target.style.height = `${Math.min(event.target.scrollHeight, 160)}px`
          }}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.shiftKey) {
              event.preventDefault()
              void submit()
            }
          }}
          className="max-h-40 w-full resize-none bg-transparent px-2 text-[15px] outline-none placeholder:text-muted"
        />

        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="sm"
            isIconOnly
            aria-label="Attach image"
            onPress={open}
          >
            <Icon icon="lucide:plus" className="h-4 w-4 text-muted" />
          </Button>

          <Popover isOpen={accessOpen} onOpenChange={setAccessOpen}>
            <Popover.Trigger
              aria-label="Access mode"
              className={`hover:bg-default flex items-center gap-1.5 rounded-lg px-2 py-1.5 text-sm transition-colors ${
                accessMode.warning ? "text-warning" : "text-muted"
              }`}
            >
              <Icon icon={accessMode.icon} className="h-4 w-4" />
              {accessMode.label}
              <Icon icon="lucide:chevron-down" className="h-3.5 w-3.5" />
            </Popover.Trigger>
            <Popover.Content className="w-72 rounded-2xl border border-border p-1.5">
              <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
                {ACCESS_MODES.map((mode) => (
                  <button
                    key={mode.id}
                    type="button"
                    onClick={() => {
                      setAccessMode(mode)
                      setAccessOpen(false)
                    }}
                    className="hover:bg-default flex items-start gap-2.5 rounded-xl px-2.5 py-2 text-left transition-colors"
                  >
                    <Icon
                      icon={mode.icon}
                      className={`mt-0.5 h-4 w-4 shrink-0 ${mode.warning ? "text-warning" : "text-muted"}`}
                    />
                    <span className="flex min-w-0 flex-1 flex-col">
                      <span className="text-sm">{mode.label}</span>
                      <span className="text-xs text-muted">
                        {mode.description}
                      </span>
                    </span>
                    {mode.id === accessMode.id ? (
                      <Icon
                        icon="lucide:check"
                        className="mt-1 h-4 w-4 shrink-0"
                      />
                    ) : null}
                  </button>
                ))}
              </Popover.Dialog>
            </Popover.Content>
          </Popover>

          <div className="flex-1" />

          <Popover isOpen={modelOpen} onOpenChange={setModelOpen}>
            <Popover.Trigger
              aria-label="Model and effort"
              className="hover:bg-default flex items-center gap-1.5 rounded-lg px-2 py-1.5 text-sm transition-colors"
            >
              <ModelIcon model={selectedModel} className="h-3.5 w-3.5" />
              <span className="font-medium">{selectedModel.label}</span>
              <span className="text-muted">{effort}</span>
              <Icon
                icon="lucide:chevron-down"
                className="h-3.5 w-3.5 text-muted"
              />
            </Popover.Trigger>
            <Popover.Content className="w-64 rounded-2xl border border-border p-1.5">
              <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
                <span className="px-2.5 pt-1.5 pb-1 text-xs text-muted">
                  Models available to {agent.name}
                </span>
                {agent.modelIds.map(displayModel).map((entry) => (
                  <button
                    key={entry.id}
                    type="button"
                    onClick={() => onModelChange(entry.id)}
                    className="hover:bg-default flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors"
                  >
                    <ModelIcon model={entry} className="h-4 w-4 shrink-0" />
                    <span className="flex min-w-0 flex-1 flex-col">
                      <span>{entry.label}</span>
                      <span className="text-xs text-muted">
                        {entry.provider}
                      </span>
                    </span>
                    {entry.id === modelId ? (
                      <Icon icon="lucide:check" className="h-4 w-4 shrink-0" />
                    ) : null}
                  </button>
                ))}
                <span className="px-2.5 pt-2 pb-1 text-xs text-muted">
                  Reasoning effort
                </span>
                {EFFORTS.map((entry) => (
                  <button
                    key={entry}
                    type="button"
                    onClick={() => {
                      setEffort(entry)
                      setModelOpen(false)
                    }}
                    className="hover:bg-default flex items-center gap-2 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors"
                  >
                    <span className="min-w-0 flex-1">{entry}</span>
                    {entry === effort ? (
                      <Icon icon="lucide:check" className="h-4 w-4 shrink-0" />
                    ) : null}
                  </button>
                ))}
              </Popover.Dialog>
            </Popover.Content>
          </Popover>

          <Button
            variant="ghost"
            size="sm"
            isIconOnly
            aria-label={recording ? "Stop dictation" : "Dictate"}
            onPress={() => setRecording((value) => !value)}
          >
            <Icon
              icon="lucide:mic"
              className={`h-4 w-4 ${recording ? "text-danger" : "text-muted"}`}
            />
          </Button>
          {isStreaming ? (
            <Button
              variant="primary"
              size="sm"
              isIconOnly
              aria-label="Stop"
              onPress={onStop}
              className="rounded-full"
            >
              <Icon icon="lucide:square" className="h-3.5 w-3.5" />
            </Button>
          ) : (
            <Button
              variant="primary"
              size="sm"
              isIconOnly
              aria-label="Send"
              isDisabled={!canSend || isStreaming}
              onPress={() => void submit()}
              className="rounded-full"
            >
              <Icon icon="lucide:arrow-up" className="h-4 w-4" />
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}
