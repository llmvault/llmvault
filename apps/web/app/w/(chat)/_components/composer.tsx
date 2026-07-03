"use client"

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { Button, Spinner } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { useDropzone } from "react-dropzone"
import { $api } from "@/lib/api/hooks"
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
  requireImageDescriptionResult,
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
import { useSessionAudioTranscription } from "@/hooks/use-session-audio-transcription"
import { ComposerLineComments } from "./composer-line-comments"
import { MicrophonePermissionModal } from "./microphone-permission-modal"
import { RecordingWaveform } from "./recording-waveform"
import { displayModel, ModelIcon } from "./model-display"
import { useSessionUsageSummary } from "@/app/w/(chat)/_stores/session-runtime-store"
import { SessionSpendPill } from "./session-spend-pill"
import {
  agentDisplayName,
  channelDisplayName,
  type SidebarAgentResponse,
  type SidebarChannelResponse,
} from "@/app/w/(chat)/_lib/sidebar-data"
import type { ModelSummary } from "@/app/w/(chat)/_lib/model-options"
import { Picker, PickerButton } from "./chat-picker"
import { PickerText } from "./chat-picker-text"

const EFFORTS = ["Low", "Medium", "High"]

export function Composer({
  sessionId,
  agentId,
  modelId,
  onSend,
  placeholder = "Ask for follow-up changes",
  isStreaming = false,
  isDisabled = false,
  isSubmitting = false,
  onStop,
  attachmentsEnabled = true,
  audioEnabled = true,
  spendVisible = true,
  channelSelectable = false,
  channel,
  channels = [],
  channelsLoading = false,
  channelsError = false,
  onChannelChange,
  agentSelectable = false,
  agent,
  agents = [],
  agentsLoading = false,
  agentsError = false,
  onAgentChange,
  modelSelectable = false,
  modelIds = [],
  modelSummaries = [],
  modelsLoading = false,
  modelsError = false,
  onModelChange,
}: {
  sessionId: string
  agentId: string
  modelId: string
  onSend: (
    text: string,
    attachments: ImageAttachmentMetadata[],
    codeLineComments: CodeLineCommentPayload[],
    effort: string
  ) => boolean | void | Promise<boolean | void>
  placeholder?: string
  isStreaming?: boolean
  isDisabled?: boolean
  isSubmitting?: boolean
  onStop?: () => void
  // Session-only affordances; turn off where no session exists yet.
  attachmentsEnabled?: boolean
  audioEnabled?: boolean
  spendVisible?: boolean
  channelSelectable?: boolean
  channel?: SidebarChannelResponse
  channels?: SidebarChannelResponse[]
  channelsLoading?: boolean
  channelsError?: boolean
  onChannelChange?: (channel: SidebarChannelResponse) => void
  agentSelectable?: boolean
  agent?: SidebarAgentResponse
  agents?: SidebarAgentResponse[]
  agentsLoading?: boolean
  agentsError?: boolean
  onAgentChange?: (agent: SidebarAgentResponse) => void
  modelSelectable?: boolean
  modelIds?: string[]
  modelSummaries?: ModelSummary[]
  modelsLoading?: boolean
  modelsError?: boolean
  onModelChange?: (modelId: string) => void
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
  const effort = workspace.composer.effort
  const setEffortValue = useSessionWorkspaceStore(
    (state) => state.setComposerEffort
  )
  const [channelOpen, setChannelOpen] = useState(false)
  const [agentOpen, setAgentOpen] = useState(false)
  const [modelOpen, setModelOpen] = useState(false)
  const lineComments = useCodeLineComments()
  const lineCommentActions = useCodeLineCommentActions()
  const attachmentDescriptions = workspace.composer.attachmentDescriptions
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)
  const describedUploadsRef = useRef<Set<string>>(new Set())
  const usage = useSessionUsageSummary(sessionId)
  const { mutateAsync: describeDriveImage } = $api.useMutation(
    "post",
    "/v1/images/describe"
  )
  const { uploads, addFiles, retryUpload, removeUpload } =
    useOrgDriveFileUploads({ agentId, sessionId })

  const selectedModel = displayModel(modelId, modelSummaries)
  const modelOptions = useMemo(
    () => modelIds.map((id) => displayModel(id, modelSummaries)),
    [modelIds, modelSummaries]
  )
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
    !isDisabled &&
    !isSubmitting &&
    !hasPendingAttachment &&
    !hasFailedAttachment &&
    (!channelSelectable || Boolean(channel?.id)) &&
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
        const description = requireImageDescriptionResult(
          await describeDriveImage({
            body: {
              drive_asset_id: upload.asset.id,
              detail_level: "high",
              session_id: sessionId,
            },
          })
        )
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
    [describeDriveImage, sessionId, setAttachmentDescriptions]
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
      if (isDisabled) return
      addFiles(files)
    },
    [addFiles, isDisabled]
  )

  const { getRootProps, getInputProps, open, isDragActive, isDragReject } =
    useDropzone({
      accept: { "image/*": [] },
      disabled: isDisabled || !attachmentsEnabled,
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
      if (isDisabled || isSubmitting) {
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
          sendingCodeLineComments,
          effort
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
      effort,
      isDisabled,
      isSubmitting,
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
    isStreaming: isStreaming || isDisabled,
    onTranscript: handleRecordingTranscript,
    transcription: useSessionAudioTranscription({ agentId, sessionId }),
  })

  const submit = async () => {
    if (isDisabled || isSubmitting) return
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
            disabled={isDisabled}
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
            className="max-h-16 min-h-16 w-full resize-none overflow-y-auto bg-transparent px-2 text-[15px] outline-none placeholder:text-muted disabled:cursor-not-allowed disabled:opacity-60"
          />

          <div className="flex items-center gap-1">
            {attachmentsEnabled ? (
              <Button
                variant="ghost"
                size="sm"
                isIconOnly
                aria-label="Attach image"
                isDisabled={isDisabled}
                onPress={open}
              >
                <AppIcon icon="plus" className="h-4 w-4 text-muted" />
              </Button>
            ) : null}
            {spendVisible ? <SessionSpendPill usage={usage} /> : null}
            {channelSelectable ? (
              <Picker
                open={channelOpen}
                setOpen={setChannelOpen}
                label="Select channel"
                icon="hash"
                value={
                  channelsLoading
                    ? "Loading channels"
                    : channel
                      ? channelDisplayName(channel)
                      : "Select channel"
                }
                width="w-64"
              >
                {channelsLoading ? (
                  <PickerText>Loading channels</PickerText>
                ) : channelsError ? (
                  <PickerText>Could not load channels</PickerText>
                ) : channels.length ? (
                  channels.map((entry) => (
                    <PickerButton
                      key={entry.id ?? channelDisplayName(entry)}
                      icon="hash"
                      selected={entry.id === channel?.id}
                      onPress={() => {
                        onChannelChange?.(entry)
                        setChannelOpen(false)
                      }}
                    >
                      {channelDisplayName(entry)}
                    </PickerButton>
                  ))
                ) : (
                  <PickerText>No channels</PickerText>
                )}
              </Picker>
            ) : null}
            {agentSelectable ? (
              <Picker
                open={agentOpen}
                setOpen={setAgentOpen}
                label="Select agent"
                agent={agent}
                value={
                  agentsLoading
                    ? "Loading agents"
                    : agent
                      ? agentDisplayName(agent)
                      : "Select agent"
                }
                width="w-72"
              >
                {agentsLoading ? (
                  <PickerText>Loading agents</PickerText>
                ) : agentsError ? (
                  <PickerText>Could not load agents</PickerText>
                ) : agents.length ? (
                  agents.map((entry) => (
                    <PickerButton
                      key={entry.id ?? agentDisplayName(entry)}
                      agent={entry}
                      selected={entry.id === agent?.id}
                      onPress={() => {
                        onAgentChange?.(entry)
                        setAgentOpen(false)
                      }}
                    >
                      {agentDisplayName(entry)}
                    </PickerButton>
                  ))
                ) : (
                  <PickerText>No agents</PickerText>
                )}
              </Picker>
            ) : null}

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

                {modelSelectable ? (
                  <Picker
                    open={modelOpen}
                    setOpen={setModelOpen}
                    label="Select model"
                    model={selectedModel}
                    value={selectedModel.label}
                    suffix={effort}
                    width="w-64"
                  >
                    <span className="px-2.5 pt-1.5 pb-1 text-xs text-muted">
                      Models
                    </span>
                    {modelsLoading && modelOptions.length === 0 ? (
                      <PickerText>Loading models</PickerText>
                    ) : modelsError && modelOptions.length === 0 ? (
                      <PickerText>Could not load models</PickerText>
                    ) : modelOptions.length ? (
                      modelOptions.map((entry) => (
                        <PickerButton
                          key={entry.id}
                          model={entry}
                          selected={entry.id === modelId}
                          onPress={() => onModelChange?.(entry.id)}
                        >
                          {entry.label}
                        </PickerButton>
                      ))
                    ) : (
                      <PickerText>No models</PickerText>
                    )}
                    <span className="px-2.5 pt-2 pb-1 text-xs text-muted">
                      Reasoning effort
                    </span>
                    {EFFORTS.map((entry) => (
                      <PickerButton
                        key={entry}
                        selected={entry === effort}
                        onPress={() => {
                          setEffortValue(sessionId, entry)
                          setModelOpen(false)
                        }}
                      >
                        {entry}
                      </PickerButton>
                    ))}
                  </Picker>
                ) : (
                  <div
                    aria-label={`Model: ${selectedModel.label}`}
                    className="flex min-w-0 items-center gap-1.5 px-2 py-1.5 text-sm text-muted"
                  >
                    <ModelIcon model={selectedModel} className="h-4 w-4" />
                    <span className="max-w-40 truncate font-medium text-foreground">
                      {selectedModel.label}
                    </span>
                  </div>
                )}
              </>
            )}

            {!audioEnabled ? null : isTranscribingRecording ? (
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
                isDisabled={
                  isDisabled || isProcessingStartRecording || isStreaming
                }
                onPress={() => void toggleRecording()}
              >
                <AppIcon
                  icon={isRecordingInProgress ? "square" : "mic"}
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
                isDisabled={isDisabled}
                onPress={onStop}
                className="rounded-full"
              >
                <AppIcon icon="square" className="h-3.5 w-3.5" />
              </Button>
            ) : (
              <Button
                variant="primary"
                size="sm"
                isIconOnly
                aria-label="Send"
                isDisabled={
                  isDisabled ||
                  isSubmitting ||
                  isProcessingStartRecording ||
                  isTranscribingRecording ||
                  (!isRecordingInProgress && !canSend)
                }
                onPress={() => void submit()}
                className="rounded-full"
              >
                {isSubmitting ? (
                  <Spinner color="current" size="sm" />
                ) : (
                  <AppIcon icon="arrow-up" className="h-4 w-4" />
                )}
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

function discardDraftUploads(uploads: WorkspaceUploadItem[]) {
  for (const upload of uploads) {
    if (upload.previewUrl?.startsWith("blob:")) {
      URL.revokeObjectURL(upload.previewUrl)
    }
    void deleteDraftAttachmentBlob(upload.blobKey)
  }
}
