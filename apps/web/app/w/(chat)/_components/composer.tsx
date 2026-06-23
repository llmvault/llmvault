"use client"

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { Button, Popover } from "@heroui/react"
import { Icon } from "@iconify/react"
import { useDropzone } from "react-dropzone"
import { cn } from "@/lib/utils"
import {
  useOrgDriveFileUploads,
  type OrgDriveUploadItem,
} from "@/hooks/use-org-drive-file-uploads"
import {
  selectSessionWorkspace,
  useSessionWorkspaceStore,
} from "@/app/w/(chat)/_stores/session-workspace-store"
import type { Agent } from "@/app/w/(chat)/_lib/agents"
import {
  attachmentMetadataFromDescription,
  describeDriveImage,
  type ImageAttachmentMetadata,
} from "@/app/w/(chat)/_lib/image-attachments"
import {
  AttachmentPreviewTray,
  type ComposerImageAttachment,
} from "./composer-attachments"
import {
  codeLineCommentPayloads,
  useCodeLineCommentActions,
  useCodeLineComments,
} from "@/app/w/(chat)/_components/line-comments"
import type { CodeLineCommentPayload } from "@/app/w/(chat)/_lib/code-line-comments"
import { ComposerLineComments } from "./composer-line-comments"
import { displayModel, ModelIcon } from "./model-display"

const EFFORTS = ["Low", "Medium", "High"]

export function Composer({
  sessionId,
  agent,
  agentId,
  modelId,
  onModelChange,
  onSend,
  placeholder = "Ask for follow-up changes",
  isStreaming = false,
  onStop,
}: {
  sessionId: string
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
  const workspace = useSessionWorkspaceStore((state) =>
    selectSessionWorkspace(state, sessionId)
  )
  const value = workspace.composer.text
  const effort = workspace.composer.effort
  const setValue = useSessionWorkspaceStore((state) => state.setComposerText)
  const setEffortValue = useSessionWorkspaceStore(
    (state) => state.setComposerEffort
  )
  const setAttachmentDescriptions = useSessionWorkspaceStore(
    (state) => state.setAttachmentDescriptions
  )
  const [modelOpen, setModelOpen] = useState(false)
  const [recording, setRecording] = useState(false)
  const lineComments = useCodeLineComments()
  const lineCommentActions = useCodeLineCommentActions()
  const attachmentDescriptions = workspace.composer.attachmentDescriptions
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)
  const describedUploadsRef = useRef<Set<string>>(new Set())
  const { uploads, addFiles, retryUpload, removeUpload, clearUploads } =
    useOrgDriveFileUploads({ agentId, sessionId })

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
    setAttachmentDescriptions(sessionId, (current) => ({
      ...current,
      [upload.id]: { status: "describing" },
    }))
    try {
      const description = await describeDriveImage(upload.asset.id)
      const metadata = attachmentMetadataFromDescription(
        upload.asset,
        description
      )
      setAttachmentDescriptions(sessionId, (current) => ({
        ...current,
        [upload.id]: { status: "ready", metadata },
      }))
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Image processing failed"
      setAttachmentDescriptions(sessionId, (current) => ({
        ...current,
        [upload.id]: { status: "error", error: message },
      }))
    }
  }, [sessionId, setAttachmentDescriptions])

  useEffect(() => {
    for (const upload of uploads) {
      if (upload.status !== "uploaded" || !upload.asset) continue
      if (attachmentDescriptions[upload.id]?.status) continue
      if (describedUploadsRef.current.has(upload.id)) continue
      describedUploadsRef.current.add(upload.id)
      if (!upload.asset.content_type.startsWith("image/")) {
        window.queueMicrotask(() => {
          setAttachmentDescriptions(sessionId, (current) => ({
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
  }, [attachmentDescriptions, describeUpload, sessionId, setAttachmentDescriptions, uploads])

  const retryAttachment = (attachment: ComposerImageAttachment) => {
    const id = attachment.upload.id
    setAttachmentDescriptions(sessionId, (current) => {
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
    setAttachmentDescriptions(sessionId, (current) => {
      const next = { ...current }
      delete next[id]
      return next
    })
  }

  const resetAttachments = () => {
    clearUploads()
    describedUploadsRef.current.clear()
    setAttachmentDescriptions(sessionId, () => ({}))
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
    setValue(sessionId, "")
    try {
      const sent = await onSend(
        promptText,
        sendingAttachments,
        sendingCodeLineComments
      )
      if (sent === false) {
        if (!useSessionWorkspaceStore.getState().workspaces[sessionId]?.composer.text) {
          setValue(sessionId, promptText)
        }
        return
      }
      resetAttachments()
      lineCommentActions.removeComments(sendingLineCommentIds)
      const node = textareaRef.current
      if (node) {
        node.style.height = "auto"
      }
    } catch {
      if (!useSessionWorkspaceStore.getState().workspaces[sessionId]?.composer.text) {
        setValue(sessionId, promptText)
      }
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
            setValue(sessionId, event.target.value)
            event.target.style.height = "auto"
            event.target.style.height = `${Math.min(event.target.scrollHeight, 64)}px`
          }}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.shiftKey) {
              event.preventDefault()
              void submit()
            }
          }}
          className="max-h-16 min-h-16 w-full resize-none overflow-y-auto bg-transparent px-2 text-[15px] outline-none placeholder:text-muted"
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
                      setEffortValue(sessionId, entry)
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
