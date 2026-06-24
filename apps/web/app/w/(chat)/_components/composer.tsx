"use client"

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { Button } from "@heroui/react"
import { Icon } from "@iconify/react"
import { useVoiceVisualizer } from "react-voice-visualizer"
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
import { useMicrophonePermission } from "@/hooks/use-microphone-permission"
import { useSessionAudioTranscription } from "@/hooks/use-session-audio-transcription"
import { ComposerLineComments } from "./composer-line-comments"
import { MicrophonePermissionModal } from "./microphone-permission-modal"
import { RecordingWaveform } from "./recording-waveform"
import { displayModel, ModelIcon } from "./model-display"

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
  const setAttachmentDescriptions = useSessionWorkspaceStore(
    (state) => state.setAttachmentDescriptions
  )
  const lineComments = useCodeLineComments()
  const lineCommentActions = useCodeLineCommentActions()
  const [micPromptOpen, setMicPromptOpen] = useState(false)
  const attachmentDescriptions = workspace.composer.attachmentDescriptions
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)
  const describedUploadsRef = useRef<Set<string>>(new Set())
  const recordingMimeTypeRef = useRef("")
  const recordingInProgressRef = useRef(false)
  const recordingDurationRef = useRef(0)
  const lastLoggedRecordingRef = useRef<Blob | null>(null)
  const recordingStartedAtRef = useRef<number | null>(null)
  const stopRecordingRef = useRef<() => void>(() => {})
  const { uploads, addFiles, retryUpload, removeUpload, clearUploads } =
    useOrgDriveFileUploads({ agentId, sessionId })
  const { hasGrantedMicrophonePermission, setMicPermissionGranted } =
    useMicrophonePermission()
  const recorderControls = useVoiceVisualizer({
    onStartRecording: () => setMicPermissionGranted(true),
    shouldHandleBeforeUnload: false,
  })
  const {
    audioData,
    clearCanvas,
    error: recordingError,
    formattedRecordingTime,
    isProcessingStartRecording,
    isRecordingInProgress,
    mediaRecorder,
    recordedBlob,
    recordingTime,
    startRecording,
    stopRecording,
  } = recorderControls
  const {
    mutateAsync: transcribeRecording,
    isPending: isTranscribingRecording,
  } = useSessionAudioTranscription({ agentId, sessionId })

  const selectedModel = displayModel(modelId)
  const recordingActive = isRecordingInProgress || isProcessingStartRecording
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
    !isTranscribingRecording &&
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

  useEffect(() => {
    recordingInProgressRef.current = isRecordingInProgress
    stopRecordingRef.current = stopRecording
  }, [isRecordingInProgress, stopRecording])

  useEffect(() => {
    return () => {
      if (recordingInProgressRef.current) {
        stopRecordingRef.current()
      }
    }
  }, [])

  useEffect(() => {
    if (!mediaRecorder) return
    recordingMimeTypeRef.current = mediaRecorder.mimeType
  }, [mediaRecorder])

  useEffect(() => {
    if (isRecordingInProgress) {
      recordingStartedAtRef.current = Date.now()
    }
  }, [isRecordingInProgress])

  useEffect(() => {
    if (recordingTime > 0) {
      recordingDurationRef.current = recordingTime
    }
  }, [recordingTime])

  useEffect(() => {
    if (!recordedBlob || lastLoggedRecordingRef.current === recordedBlob) return

    lastLoggedRecordingRef.current = recordedBlob
    const url = URL.createObjectURL(recordedBlob)
    const startedAt = recordingStartedAtRef.current
    const elapsedMs = startedAt
      ? Date.now() - startedAt
      : recordingDurationRef.current
    const mimeType =
      recordedBlob.type || recordingMimeTypeRef.current || "audio/webm"
    console.warn("Audio recording complete", {
      blob: recordedBlob,
      blobUrl: url,
      durationMs: Math.max(recordingDurationRef.current, elapsedMs),
      mimeType,
      sessionId,
      size: recordedBlob.size,
      startedAt: startedAt ? new Date(startedAt).toISOString() : null,
      stoppedAt: new Date().toISOString(),
    })
    void transcribeRecording({ blob: recordedBlob, mimeType })
      .then(({ asset, text }) => {
        console.warn("Audio transcription complete", {
          driveAssetId: asset.id,
          text,
        })
        if (!text.trim()) return
        const currentText =
          useSessionWorkspaceStore.getState().workspaces[sessionId]?.composer
            .text ?? ""
        setValue(sessionId, appendTranscriptToComposer(currentText, text))
        window.requestAnimationFrame(() => {
          const node = textareaRef.current
          if (!node) return
          node.style.height = "auto"
          node.style.height = `${Math.min(node.scrollHeight, 64)}px`
        })
      })
      .catch((error: unknown) =>
        console.error("Audio transcription failed", error)
      )

    return () => URL.revokeObjectURL(url)
  }, [recordedBlob, sessionId, setValue, transcribeRecording])

  useEffect(() => {
    if (!recordingError) return
    console.error("Audio recording failed", recordingError)
  }, [recordingError])

  const startRecordingFromCurrentState = () => {
    clearCanvas()
    recordingDurationRef.current = 0
    recordingMimeTypeRef.current = ""
    lastLoggedRecordingRef.current = null
    recordingStartedAtRef.current = Date.now()
    startRecording()
  }

  const toggleRecording = async () => {
    if (isRecordingInProgress) {
      recordingMimeTypeRef.current =
        mediaRecorder?.mimeType || recordingMimeTypeRef.current
      stopRecording()
      return
    }
    if (isProcessingStartRecording || isStreaming || isTranscribingRecording) {
      return
    }
    if (await hasGrantedMicrophonePermission()) {
      startRecordingFromCurrentState()
      return
    }
    setMicPromptOpen(true)
  }

  const startRecordingFromPrompt = () => {
    setMicPromptOpen(false)
    startRecordingFromCurrentState()
  }

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
        if (
          !useSessionWorkspaceStore.getState().workspaces[sessionId]?.composer
            .text
        ) {
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
      if (
        !useSessionWorkspaceStore.getState().workspaces[sessionId]?.composer
          .text
      ) {
        setValue(sessionId, promptText)
      }
    }
  }

  return (
    <>
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

            {recordingActive || isTranscribingRecording ? (
              <div className="flex min-w-0 flex-1 items-center gap-3 pl-2">
                <div className="h-9 min-w-0 flex-1 overflow-hidden">
                  {isRecordingInProgress ? (
                    <RecordingWaveform active audioData={audioData} />
                  ) : (
                    <div className="bg-surface-secondary h-full w-full animate-pulse rounded-full" />
                  )}
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

            <Button
              variant="ghost"
              size="sm"
              isIconOnly
              aria-label={
                isRecordingInProgress ? "Stop recording" : "Record audio"
              }
              isDisabled={
                isProcessingStartRecording ||
                isStreaming ||
                isTranscribingRecording
              }
              onPress={() => void toggleRecording()}
            >
              <Icon
                icon={isRecordingInProgress ? "lucide:square" : "lucide:mic"}
                className={`h-4 w-4 ${isRecordingInProgress ? "text-danger" : "text-muted"}`}
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

      <MicrophonePermissionModal
        open={micPromptOpen}
        onOpenChange={setMicPromptOpen}
        onConfirm={startRecordingFromPrompt}
      />
    </>
  )
}
