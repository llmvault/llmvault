"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { useVoiceVisualizer } from "react-voice-visualizer"
import { useMicrophonePermission } from "@/hooks/use-microphone-permission"
import { useSessionAudioTranscription } from "@/hooks/use-session-audio-transcription"

export type RecordingTranscriptIntent = "edit" | "send"

interface UseComposerAudioRecordingOptions {
  agentId: string
  isStreaming: boolean
  onTranscript: (
    text: string,
    intent: RecordingTranscriptIntent
  ) => Promise<void> | void
  sessionId: string
}

export function useComposerAudioRecording({
  agentId,
  isStreaming,
  onTranscript,
  sessionId,
}: UseComposerAudioRecordingOptions) {
  const [micPromptOpen, setMicPromptOpen] = useState(false)
  const completionIntentRef = useRef<RecordingTranscriptIntent>("edit")
  const recordingMimeTypeRef = useRef("")
  const recordingInProgressRef = useRef(false)
  const recordingDurationRef = useRef(0)
  const lastLoggedRecordingRef = useRef<Blob | null>(null)
  const recordingStartedAtRef = useRef<number | null>(null)
  const stopRecordingRef = useRef<() => void>(() => {})
  const onTranscriptRef = useRef(onTranscript)
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

  useEffect(() => {
    onTranscriptRef.current = onTranscript
  }, [onTranscript])

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
    if (!recordingError) return
    console.error("Audio recording failed", recordingError)
  }, [recordingError])

  useEffect(() => {
    if (!recordedBlob || lastLoggedRecordingRef.current === recordedBlob) return

    lastLoggedRecordingRef.current = recordedBlob
    const intent = completionIntentRef.current
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
      .then(async ({ asset, text }) => {
        console.warn("Audio transcription complete", {
          driveAssetId: asset.id,
          text,
        })
        if (!text.trim()) return
        await onTranscriptRef.current(text, intent)
      })
      .catch((error: unknown) =>
        console.error("Audio transcription failed", error)
      )
      .finally(() => {
        completionIntentRef.current = "edit"
      })

    return () => URL.revokeObjectURL(url)
  }, [recordedBlob, sessionId, transcribeRecording])

  const startRecordingFromCurrentState = useCallback(() => {
    clearCanvas()
    completionIntentRef.current = "edit"
    recordingDurationRef.current = 0
    recordingMimeTypeRef.current = ""
    lastLoggedRecordingRef.current = null
    recordingStartedAtRef.current = Date.now()
    startRecording()
  }, [clearCanvas, startRecording])

  const stopRecordingWithIntent = useCallback(
    (intent: RecordingTranscriptIntent) => {
      if (!isRecordingInProgress) return
      completionIntentRef.current = intent
      recordingMimeTypeRef.current =
        mediaRecorder?.mimeType || recordingMimeTypeRef.current
      stopRecording()
    },
    [isRecordingInProgress, mediaRecorder, stopRecording]
  )

  const toggleRecording = useCallback(async () => {
    if (isRecordingInProgress) {
      stopRecordingWithIntent("edit")
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
  }, [
    hasGrantedMicrophonePermission,
    isProcessingStartRecording,
    isRecordingInProgress,
    isStreaming,
    isTranscribingRecording,
    startRecordingFromCurrentState,
    stopRecordingWithIntent,
  ])

  const startRecordingFromPrompt = useCallback(() => {
    setMicPromptOpen(false)
    startRecordingFromCurrentState()
  }, [startRecordingFromCurrentState])

  const sendRecordingAfterTranscription = useCallback(() => {
    stopRecordingWithIntent("send")
  }, [stopRecordingWithIntent])

  return {
    audioData,
    formattedRecordingTime,
    isProcessingStartRecording,
    isRecordingInProgress,
    isTranscribingRecording,
    micPromptOpen,
    recordingActive: isRecordingInProgress || isProcessingStartRecording,
    sendRecordingAfterTranscription,
    setMicPromptOpen,
    startRecordingFromPrompt,
    toggleRecording,
  }
}
