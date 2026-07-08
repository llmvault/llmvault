"use client"

import { useMemo, useState } from "react"
import { useSessionWorkspaceStore } from "@/app/w/(chat)/_stores/session-workspace-store"
import type { ModelSummary } from "@/app/w/(chat)/_lib/model-options"
import { displayModel, type DisplayModel } from "./model-display"
import { Picker, PickerButton } from "./chat-picker"
import { PickerText } from "./chat-picker-text"

const EFFORTS = ["Low", "Medium", "High"]

export function ComposerModelPicker({
  sessionId,
  modelId,
  modelIds,
  modelSummaries,
  modelsLoading,
  modelsError,
  selectedModel,
  effort,
  onModelChange,
  avatarOnly = false,
}: {
  sessionId: string
  modelId: string
  modelIds: string[]
  modelSummaries: ModelSummary[]
  modelsLoading: boolean
  modelsError: boolean
  selectedModel: DisplayModel
  effort: string
  onModelChange?: (modelId: string) => void
  avatarOnly?: boolean
}) {
  const setEffortValue = useSessionWorkspaceStore(
    (state) => state.setComposerEffort
  )
  const [modelOpen, setModelOpen] = useState(false)
  const [modelQuery, setModelQuery] = useState("")
  const modelOptions = useMemo(
    () => modelIds.map((id) => displayModel(id, modelSummaries)),
    [modelIds, modelSummaries]
  )
  const filteredModelOptions = useMemo(() => {
    const query = modelQuery.trim().toLowerCase()
    if (!query) return modelOptions
    return modelOptions.filter(
      (entry) =>
        entry.label.toLowerCase().includes(query) ||
        entry.id.toLowerCase().includes(query)
    )
  }, [modelOptions, modelQuery])

  return (
    <Picker
      open={modelOpen}
      setOpen={(open) => {
        setModelOpen(open)
        if (!open) setModelQuery("")
      }}
      label="Select model"
      model={selectedModel}
      value={selectedModel.label}
      suffix={effort}
      avatarOnly={avatarOnly}
      width="w-80"
    >
      <input
        type="text"
        value={modelQuery}
        onChange={(event) => setModelQuery(event.target.value)}
        placeholder="Search models"
        aria-label="Search models"
        className="mx-1 mt-0.5 mb-1 rounded-lg border border-border bg-transparent px-2 py-1.5 text-sm outline-none placeholder:text-muted focus:border-primary"
      />
      <div className="flex max-h-80 flex-col gap-0.5 overflow-y-auto">
        {modelsLoading && modelOptions.length === 0 ? (
          <PickerText>Loading models</PickerText>
        ) : modelsError && modelOptions.length === 0 ? (
          <PickerText>Could not load models</PickerText>
        ) : filteredModelOptions.length ? (
          filteredModelOptions.map((entry) => (
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
          <PickerText>No matching models</PickerText>
        )}
      </div>
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
  )
}
