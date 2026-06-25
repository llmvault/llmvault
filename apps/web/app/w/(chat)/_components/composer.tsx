"use client"

import { useCallback, useEffect, useMemo, useRef } from "react"
import { Button, Spinner } from "@heroui/react"
import { Icon } from "@iconify/react"
import { AnimatePresence, motion } from "motion/react"
import { useDropzone } from "react-dropzone"
import { cn } from "@/lib/utils"
import {
  useOrgDriveFileUploads,
  type OrgDriveUploadItem,
} from "@/hooks/use-org-drive-file-uploads"
import {
  deleteDraftAttachmentBlob,
  selectSessionWorkspace,
  useSessionWorkspaceStore,
  type WorkspaceUploadItem,
} from "@/app/w/(chat)/_stores/session-workspace-store"
import {
  attachmentMetadataFromDescription,
  describeDriveImage,
  type ImageAttachmentMetadata,
} from "@/app/w/(chat)/_lib/image-attachments"
import { appendTranscriptToComposer } from "@/app/w/(chat)/_lib/audio-transcriptions"
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
import {
  useComposerAudioRecording,
  type RecordingTranscriptIntent,
} from "@/hooks/use-composer-audio-recording"
import { ComposerLineComments } from "./composer-line-comments"
import { MicrophonePermissionModal } from "./microphone-permission-modal"
import { RecordingWaveform } from "./recording-waveform"
import { displayModel, ModelIcon } from "./model-display"
import { useSessionUsageSummary } from "@/app/w/(chat)/_stores/session-runtime-store"
import {
  formatSessionCostUSD,
  type SessionUsageSummary,
} from "@/app/w/(chat)/_lib/session-usage"

export function Composer({
  sessionId,
  agentId,
  modelId,
  onSend,
  placeholder = "Ask for follow-up changes",
  isStreaming = false,
  onStop,
}: {
  sessionId: string
  agentId: string
  modelId: string
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
  const setValue = useSessionWorkspaceStore((state) => state.setComposerText)
  const clearComposerAfterSend = useSessionWorkspaceStore(
    (state) => state.clearComposerAfterSend
  )
  const setComposerUploads = useSessionWorkspaceStore(
    (state) => state.setComposerUploads
  )
  const setAttachmentDescriptions = useSessionWorkspaceStore(
    (state) => state.setAttachmentDescriptions
  )
  const setLineComments = useSessionWorkspaceStore(
    (state) => state.setLineComments
  )
  const lineComments = useCodeLineComments()
  const lineCommentActions = useCodeLineCommentActions()
  const attachmentDescriptions = workspace.composer.attachmentDescriptions
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)
  const describedUploadsRef = useRef<Set<string>>(new Set())
  const usage = useSessionUsageSummary(sessionId)
  const { uploads, addFiles, retryUpload, removeUpload } =
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

  const describeUpload = useCallback(
    async (upload: OrgDriveUploadItem) => {
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
    },
    [sessionId, setAttachmentDescriptions]
  )

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
  }, [
    attachmentDescriptions,
    describeUpload,
    sessionId,
    setAttachmentDescriptions,
    uploads,
  ])

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

  const resizeTextarea = useCallback(() => {
    window.requestAnimationFrame(() => {
      const node = textareaRef.current
      if (!node) return
      node.style.height = "auto"
      node.style.height = `${Math.min(node.scrollHeight, 64)}px`
    })
  }, [])

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

  const sendComposerDraft = useCallback(
    async (text: string) => {
      const promptText = text.trim()
      const sendingAttachments = readyAttachments
      const sendingUploads = workspace.composer.uploads
      const sendingAttachmentDescriptions = attachmentDescriptions
      const sendingLineComments = lineComments
      if (
        !promptText &&
        sendingAttachments.length === 0 &&
        sendingLineComments.length === 0
      ) {
        return false
      }
      const sendingLineCommentIds = sendingLineComments.map(
        (comment) => comment.id
      )
      const sendingCodeLineComments =
        codeLineCommentPayloads(sendingLineComments)
      const restoreDraft = () => {
        const current =
          useSessionWorkspaceStore.getState().workspaces[sessionId]
        const currentComposer = current?.composer
        if (!currentComposer?.text) {
          setValue(sessionId, promptText)
        }
        if (!currentComposer?.uploads.length) {
          setComposerUploads(sessionId, () => sendingUploads)
        }
        if (
          !Object.keys(currentComposer?.attachmentDescriptions ?? {}).length
        ) {
          setAttachmentDescriptions(
            sessionId,
            () => sendingAttachmentDescriptions
          )
        }
        if (!current?.lineComments.length) {
          setLineComments(sessionId, sendingLineComments)
        }
      }

      clearComposerAfterSend(sessionId)
      lineCommentActions.removeComments(sendingLineCommentIds)
      describedUploadsRef.current.clear()
      try {
        const sent = await onSend(
          promptText,
          sendingAttachments,
          sendingCodeLineComments
        )
        if (sent === false) {
          restoreDraft()
          return false
        }
        discardDraftUploads(sendingUploads)
        const node = textareaRef.current
        if (node) {
          node.style.height = "auto"
        }
        return true
      } catch {
        restoreDraft()
        return false
      }
    },
    [
      attachmentDescriptions,
      clearComposerAfterSend,
      lineCommentActions,
      lineComments,
      onSend,
      readyAttachments,
      sessionId,
      setAttachmentDescriptions,
      setComposerUploads,
      setLineComments,
      setValue,
      workspace.composer.uploads,
    ]
  )

  const handleRecordingTranscript = useCallback(
    async (text: string, intent: RecordingTranscriptIntent) => {
      const currentText =
        useSessionWorkspaceStore.getState().workspaces[sessionId]?.composer
          .text ?? ""
      const nextText = appendTranscriptToComposer(currentText, text)
      if (intent === "send" && !hasPendingAttachment && !hasFailedAttachment) {
        if (await sendComposerDraft(nextText)) return
      }
      setValue(sessionId, nextText)
      resizeTextarea()
    },
    [
      hasFailedAttachment,
      hasPendingAttachment,
      resizeTextarea,
      sendComposerDraft,
      sessionId,
      setValue,
    ]
  )

  const {
    audioData,
    formattedRecordingTime,
    isProcessingStartRecording,
    isRecordingInProgress,
    isTranscribingRecording,
    micPromptOpen,
    recordingActive,
    sendRecordingAfterTranscription,
    setMicPromptOpen,
    startRecordingFromPrompt,
    toggleRecording,
  } = useComposerAudioRecording({
    agentId,
    isStreaming,
    onTranscript: handleRecordingTranscript,
    sessionId,
  })

  const submit = async () => {
    if (isRecordingInProgress) {
      sendRecordingAfterTranscription()
      return
    }
    if (
      !canSend ||
      isProcessingStartRecording ||
      isStreaming ||
      isTranscribingRecording
    ) {
      return
    }
    await sendComposerDraft(value)
  }

  return (
    <>
      <div className="mx-auto w-full max-w-3xl px-4 pb-4">
        <div
          {...getRootProps({
            className: cn(
              "flex flex-col gap-2 rounded-3xl border bg-surface px-3 pt-3 pb-2 shadow-sm transition-colors",
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
            <SessionSpendPill usage={usage} />

            {recordingActive ? (
              <div className="flex min-w-0 flex-1 items-center gap-3 pl-2">
                <div className="h-9 min-w-0 flex-1 overflow-hidden">
                  <RecordingWaveform
                    active={isRecordingInProgress}
                    audioData={audioData}
                  />
                </div>
                <span className="shrink-0 text-sm font-medium text-muted tabular-nums">
                  {formattedRecordingTime || "00:00"}
                </span>
              </div>
            ) : (
              <>
                <div className="flex-1" />

                <div
                  aria-label={`Model: ${selectedModel.label}`}
                  className="flex min-w-0 items-center gap-1.5 px-2 py-1.5 text-sm text-muted"
                >
                  <ModelIcon model={selectedModel} className="h-4 w-4" />
                  <span className="max-w-40 truncate font-medium text-foreground">
                    {selectedModel.label}
                  </span>
                </div>
              </>
            )}

            {isTranscribingRecording ? (
              <Button
                variant="ghost"
                size="sm"
                isIconOnly
                aria-label="Transcribing audio"
                isDisabled
              >
                <Spinner color="current" size="sm" className="text-muted" />
              </Button>
            ) : (
              <Button
                variant="ghost"
                size="sm"
                isIconOnly
                aria-label={
                  isRecordingInProgress ? "Stop recording" : "Record audio"
                }
                isDisabled={isProcessingStartRecording || isStreaming}
                onPress={() => void toggleRecording()}
              >
                <Icon
                  icon={isRecordingInProgress ? "lucide:square" : "lucide:mic"}
                  className={`h-4 w-4 ${isRecordingInProgress ? "text-danger" : "text-muted"}`}
                />
              </Button>
            )}
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
                isDisabled={
                  isProcessingStartRecording ||
                  isTranscribingRecording ||
                  (!isRecordingInProgress && !canSend)
                }
                onPress={() => void submit()}
                className="rounded-full"
              >
                <Icon icon="lucide:arrow-up" className="h-4 w-4" />
              </Button>
            )}
          </div>
        </div>
      </div>

      <MicrophonePermissionModal
        open={micPromptOpen}
        onOpenChange={setMicPromptOpen}
        onConfirm={startRecordingFromPrompt}
      />
    </>
  )
}

function SessionSpendPill({ usage }: { usage?: SessionUsageSummary }) {
  const costUsd = usage?.costUsd ?? 0
  const credits = usage?.credits ?? 0
  const formattedCredits = credits.toLocaleString("en-NG")
  const formattedCost = formatSessionCostUSD(costUsd)

  return (
    <div
      aria-label={`Session spend: ${formattedCredits} credits, ${formattedCost}`}
      className="flex h-8 shrink-0 items-center gap-1.5 rounded-full border border-transparent px-2 text-xs text-muted"
    >
      <Icon icon="lucide:coins" className="h-3.5 w-3.5" />
      <AnimatedSpendNumber value={formattedCredits} />
      <AnimatedSpendNumber
        value={formattedCost}
        className="hidden sm:inline-grid"
      />
    </div>
  )
}

function AnimatedSpendNumber({
  value,
  className,
}: {
  value: string
  className?: string
}) {
  return (
    <span
      className={cn(
        "relative inline-grid h-4 overflow-hidden tabular-nums",
        className
      )}
    >
      <span className="invisible col-start-1 row-start-1 whitespace-nowrap">
        {value}
      </span>
      <AnimatePresence initial={false} mode="popLayout">
        <motion.span
          key={value}
          initial={{ y: -10, opacity: 0 }}
          animate={{ y: 0, opacity: 1 }}
          exit={{ y: 10, opacity: 0 }}
          transition={{ duration: 0.16, ease: "easeOut" }}
          className="col-start-1 row-start-1 whitespace-nowrap"
        >
          {value}
        </motion.span>
      </AnimatePresence>
    </span>
  )
}

function discardDraftUploads(uploads: WorkspaceUploadItem[]) {
  for (const upload of uploads) {
    if (upload.previewUrl?.startsWith("blob:")) {
      URL.revokeObjectURL(upload.previewUrl)
    }
    void deleteDraftAttachmentBlob(upload.blobKey)
  }
}
