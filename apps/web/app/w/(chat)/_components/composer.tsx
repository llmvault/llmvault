"use client"

import { useCallback, useRef } from "react"
import { Button, Spinner } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { useDropzone } from "react-dropzone"
import { cn } from "@/lib/utils"
import {
  selectSessionWorkspace,
  useSessionWorkspaceStore,
} from "@/app/w/(chat)/_stores/session-workspace-store"
import type { ImageAttachmentMetadata } from "@/app/w/(chat)/_lib/image-attachments"
import { appendTranscriptToComposer } from "@/app/w/(chat)/_lib/audio-transcriptions"
import { AttachmentPreviewTray } from "./composer-attachments"
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
import { useOrgAudioTranscription } from "@/hooks/use-org-audio-transcription"
import { ComposerLineComments } from "./composer-line-comments"
import { MicrophonePermissionModal } from "./microphone-permission-modal"
import { RecordingWaveform } from "./recording-waveform"
import { displayModel, ModelIcon } from "./model-display"
import { useSessionUsageSummary } from "@/app/w/(chat)/_stores/session-runtime-store"
import { SessionSpendPill } from "./session-spend-pill"
import type {
  SidebarAgentResponse,
  SidebarChannelResponse,
} from "@/app/w/(chat)/_lib/sidebar-data"
import type { ModelSummary } from "@/app/w/(chat)/_lib/model-options"
import { AgentSelect } from "@/components/agent-select"
import {
  discardDraftUploads,
  useComposerAttachments,
} from "./use-composer-attachments"
import { ComposerChannelPicker } from "./composer-channel-picker"
import { ComposerModelPicker } from "./composer-model-picker"

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
  sessionExists = true,
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
  // False on the new-chat screen: uploads/describe/transcription then run
  // without a session (org-scoped) and sessionId is just the draft key.
  sessionExists?: boolean
  // Session-only affordances; turn off where unwanted.
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
  const setLineComments = useSessionWorkspaceStore(
    (state) => state.setLineComments
  )
  const effort = workspace.composer.effort
  const lineComments = useCodeLineComments()
  const lineCommentActions = useCodeLineCommentActions()
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)
  const usage = useSessionUsageSummary(sessionId)

  const selectedModel = displayModel(modelId, modelSummaries)
  const {
    attachments,
    attachmentDescriptions,
    setAttachmentDescriptions,
    describedUploadsRef,
    hasPendingAttachment,
    hasFailedAttachment,
    readyAttachments,
    retryAttachment,
    removeAttachment,
    onDropAccepted,
  } = useComposerAttachments({ sessionId, agentId, sessionExists, isDisabled })
  const canSend =
    !isDisabled &&
    !isSubmitting &&
    !hasPendingAttachment &&
    !hasFailedAttachment &&
    (!channelSelectable || Boolean(channel?.id)) &&
    (value.trim().length > 0 ||
      readyAttachments.length > 0 ||
      lineComments.length > 0)

  const resizeTextarea = useCallback(() => {
    window.requestAnimationFrame(() => {
      const node = textareaRef.current
      if (!node) return
      node.style.height = "auto"
      node.style.height = `${Math.min(node.scrollHeight, 64)}px`
    })
  }, [])

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
      describedUploadsRef,
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

  const sessionTranscription = useSessionAudioTranscription({
    agentId,
    sessionId,
  })
  const orgTranscription = useOrgAudioTranscription()

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
    transcription: sessionExists ? sessionTranscription : orgTranscription,
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
                variant="secondary"
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
              <ComposerChannelPicker
                channel={channel}
                channels={channels}
                channelsLoading={channelsLoading}
                channelsError={channelsError}
                onChannelChange={onChannelChange}
              />
            ) : null}
            {agentSelectable ? (
              agentsError ? (
                <span className="px-2 py-1.5 text-sm text-muted">
                  Could not load agents
                </span>
              ) : (
                <AgentSelect
                  agents={agents}
                  selectedAgentID={agent?.id ?? ""}
                  isLoading={agentsLoading}
                  onChange={(agentID) => {
                    const entry = agents.find((item) => item.id === agentID)
                    if (entry) onAgentChange?.(entry)
                  }}
                />
              )
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
                  <ComposerModelPicker
                    sessionId={sessionId}
                    modelId={modelId}
                    modelIds={modelIds}
                    modelSummaries={modelSummaries}
                    modelsLoading={modelsLoading}
                    modelsError={modelsError}
                    selectedModel={selectedModel}
                    effort={effort}
                    onModelChange={onModelChange}
                  />
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
