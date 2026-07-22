"use client"

import { useEffect, useMemo, useState } from "react"
import Image from "next/image"
import { Button, Input, ListBox } from "@heroui/react"
import { AnimatePresence, motion, useReducedMotion } from "motion/react"
import { AppIcon } from "@/components/icon"
import { modelLogoURL } from "@/lib/model-logos"
import { cn } from "@/lib/utils"

const modelOptions = [
  {
    id: "deepseek-v4-flash",
    name: "DeepSeek V4 Flash",
    fit: "Fast, high-volume work",
  },
  {
    id: "claude-sonnet-5",
    name: "Claude Sonnet 5",
    fit: "Complex knowledge work",
  },
  {
    id: "gpt-5.4-mini",
    name: "GPT-5.4 Mini",
    fit: "Everyday agent tasks",
  },
  {
    id: "gemini-3.1-pro-preview",
    name: "Gemini 3.1 Pro",
    fit: "Long-context analysis",
  },
  {
    id: "gpt-5.3-codex",
    name: "GPT-5.3 Codex",
    fit: "Software engineering",
  },
  {
    id: "qwen3.7-plus",
    name: "Qwen 3.7 Plus",
    fit: "Open-model flexibility",
  },
  { id: "grok-4.5", name: "Grok 4.5", fit: "Deep reasoning" },
  {
    id: "mistral-small-4",
    name: "Mistral Small 4",
    fit: "Efficient multilingual work",
  },
  {
    id: "claude-opus-4.7",
    name: "Claude Opus 4.7",
    fit: "Highest-complexity work",
  },
  {
    id: "gpt-5.4-nano",
    name: "GPT-5.4 Nano",
    fit: "Lightweight routing",
  },
  {
    id: "gemini-3.5-flash",
    name: "Gemini 3.5 Flash",
    fit: "Fast multimodal work",
  },
  {
    id: "deepseek-v4-pro",
    name: "DeepSeek V4 Pro",
    fit: "Advanced reasoning",
  },
  {
    id: "qwen3.7-max",
    name: "Qwen 3.7 Max",
    fit: "Open-model reasoning",
  },
  {
    id: "kimi-k2.7-code",
    name: "Kimi K2.7 Code",
    fit: "Agentic coding",
  },
  {
    id: "mimo-v2.5-pro",
    name: "Xiaomi MiMo V2.5 Pro",
    fit: "Efficient agentic reasoning",
  },
  {
    id: "step-3.7-flash",
    name: "StepFun Step 3.7 Flash",
    fit: "Fast agent workflows",
  },
  {
    id: "ling-2.6-1t",
    name: "InclusionAI Ling 2.6 1T",
    fit: "Large-scale reasoning",
  },
  {
    id: "nemotron-3-ultra-550b-a55b",
    name: "NVIDIA Nemotron 3 Ultra",
    fit: "Complex open-model reasoning",
  },
] as const

const agentAssignments = [
  {
    id: "support",
    name: "Support agent",
    job: "High-volume ticket investigation",
    icon: "headset",
  },
  {
    id: "research",
    name: "Research agent",
    job: "Complex synthesis and decisions",
    icon: "search",
  },
  {
    id: "code-review",
    name: "Code review agent",
    job: "Pull request analysis",
    icon: "code-xml",
  },
  {
    id: "revenue",
    name: "Revenue agent",
    job: "Long account context",
    icon: "chart-spline",
  },
  {
    id: "operations",
    name: "Operations agent",
    job: "Open-model workflow routing",
    icon: "bot",
  },
] as const

export const modelAssignmentSequence = [
  {
    query: "nemotron",
    modelID: "nemotron-3-ultra-550b-a55b",
    agentID: "support",
  },
  { query: "mimo", modelID: "mimo-v2.5-pro", agentID: "research" },
  { query: "kimi", modelID: "kimi-k2.7-code", agentID: "code-review" },
  { query: "ling", modelID: "ling-2.6-1t", agentID: "revenue" },
  {
    query: "stepfun",
    modelID: "step-3.7-flash",
    agentID: "operations",
  },
  {
    query: "deepseek",
    modelID: "deepseek-v4-flash",
    agentID: "support",
  },
  { query: "claude", modelID: "claude-sonnet-5", agentID: "research" },
  { query: "codex", modelID: "gpt-5.3-codex", agentID: "code-review" },
  {
    query: "gemini",
    modelID: "gemini-3.1-pro-preview",
    agentID: "revenue",
  },
  { query: "qwen", modelID: "qwen3.7-plus", agentID: "operations" },
] as const

const staticModelAssignments = {
  support: "deepseek-v4-flash",
  research: "claude-sonnet-5",
  "code-review": "gpt-5.3-codex",
  revenue: "gemini-3.1-pro-preview",
  operations: "qwen3.7-plus",
} as const

export function updateModelAssignments(
  current: Readonly<Record<string, string>>,
  target: { agentID: string; modelID: string }
): Record<string, string> {
  return { ...current, [target.agentID]: target.modelID }
}

function ModelLogo({
  id,
  name,
  size = "md",
}: {
  id: string
  name: string
  size?: "sm" | "md"
}) {
  const logo = modelLogoURL(id)

  return (
    <span
      className={cn(
        "flex shrink-0 items-center justify-center rounded-sm border border-border bg-white p-1",
        size === "sm" ? "size-7" : "size-9"
      )}
    >
      {logo ? (
        <Image
          src={logo}
          alt=""
          width={24}
          height={24}
          className="size-full object-contain"
        />
      ) : (
        <AppIcon
          icon="brain"
          size={size === "sm" ? 14 : 18}
          className="text-muted"
        />
      )}
      <span className="sr-only">{name}</span>
    </span>
  )
}

export function ModelChoiceSection() {
  const shouldReduceMotion = useReducedMotion()
  const [cycle, setCycle] = useState(0)
  const [isOpen, setIsOpen] = useState(false)
  const [query, setQuery] = useState("")
  const [selectedModelID, setSelectedModelID] = useState<string>(
    modelOptions[0].id
  )
  const [isSelecting, setIsSelecting] = useState(false)
  const [assignedModels, setAssignedModels] = useState<Record<string, string>>(
    {}
  )

  useEffect(() => {
    if (shouldReduceMotion) return

    const target =
      modelAssignmentSequence[cycle % modelAssignmentSequence.length]
    const timers: ReturnType<typeof setTimeout>[] = []
    const schedule = (callback: () => void, delay: number) => {
      timers.push(setTimeout(callback, delay))
    }

    schedule(() => {
      setQuery("")
      setIsSelecting(false)
      setIsOpen(true)
    }, 550)

    for (let index = 1; index <= target.query.length; index += 1) {
      schedule(() => setQuery(target.query.slice(0, index)), 1150 + index * 105)
    }

    const selectedAt = 1150 + target.query.length * 105 + 650
    schedule(() => {
      setSelectedModelID(target.modelID)
      setIsSelecting(true)
    }, selectedAt)
    schedule(() => {
      setAssignedModels((current) => updateModelAssignments(current, target))
    }, selectedAt + 380)
    schedule(() => setIsOpen(false), selectedAt + 700)
    schedule(() => {
      setQuery("")
      setIsSelecting(false)
    }, selectedAt + 1000)
    schedule(() => setCycle((value) => value + 1), selectedAt + 2100)

    return () => timers.forEach(clearTimeout)
  }, [cycle, shouldReduceMotion])

  const currentModelID = shouldReduceMotion
    ? "claude-sonnet-5"
    : selectedModelID
  const currentQuery = shouldReduceMotion ? "claude" : query
  const currentIsOpen = shouldReduceMotion ? true : isOpen
  const currentIsSelecting = shouldReduceMotion ? true : isSelecting
  const currentAssignedModels = shouldReduceMotion
    ? staticModelAssignments
    : assignedModels
  const currentTargetAgentID = shouldReduceMotion
    ? "research"
    : modelAssignmentSequence[cycle % modelAssignmentSequence.length].agentID
  const selectedModel =
    modelOptions.find((model) => model.id === currentModelID) ?? modelOptions[0]
  const filteredModels = useMemo(() => {
    const normalizedQuery = currentQuery.trim().toLowerCase()
    if (!normalizedQuery) return modelOptions
    return modelOptions.filter((model) =>
      `${model.name} ${model.fit}`.toLowerCase().includes(normalizedQuery)
    )
  }, [currentQuery])

  return (
    <section
      aria-labelledby="model-choice-heading"
      className="mx-auto mt-28 w-[calc(100%-2rem)] max-w-[1300px]"
    >
      <div className="mx-auto max-w-[900px] text-center">
        <p className="text-xs font-medium tracking-[0.16em] text-muted uppercase">
          Model choice per agent
        </p>
        <h2
          id="model-choice-heading"
          className="mt-5 text-[clamp(2.25rem,5vw,4.4rem)] leading-[0.98] font-medium tracking-[-0.055em]"
        >
          Stop wasting tokens - pick the right model for the right job
        </h2>
        <p className="mx-auto mt-6 max-w-[68ch] text-base leading-7 text-muted sm:text-lg">
          Give routine work a fast, efficient model and save premium reasoning
          for the jobs that need it. Every Hivy agent can use the model that
          fits its role.
        </p>
      </div>

      <div className="mt-14 overflow-hidden rounded-sm border border-border bg-surface shadow-sm">
        <div className="grid lg:min-h-[660px] lg:grid-cols-[1.08fr_0.92fr]">
          <div className="flex items-center justify-center bg-surface-secondary p-4 sm:p-8 lg:p-10">
            <div className="w-full max-w-[680px]" aria-hidden="true" inert>
              <div className="mb-4 flex items-end justify-between gap-4">
                <div>
                  <p className="text-sm font-medium">Choose an agent model</p>
                  <p className="mt-1 text-xs text-muted">
                    Search 18 available models
                  </p>
                </div>
                <span className="text-xs text-muted">
                  {currentIsOpen
                    ? currentQuery
                      ? "Searching"
                      : "Browse models"
                    : "Model assigned"}
                </span>
              </div>

              <div className="relative min-h-[530px]">
                <Button
                  variant="secondary"
                  aria-expanded={currentIsOpen}
                  className="h-14 w-full justify-between rounded-sm border border-border bg-surface px-4 shadow-sm"
                >
                  <span className="flex min-w-0 items-center gap-3">
                    <ModelLogo
                      id={selectedModel.id}
                      name={selectedModel.name}
                      size="sm"
                    />
                    <span className="min-w-0 text-left">
                      <span className="block truncate text-sm font-medium">
                        {selectedModel.name}
                      </span>
                      <span className="block truncate text-xs text-muted">
                        {selectedModel.fit}
                      </span>
                    </span>
                  </span>
                  <motion.span
                    animate={{ rotate: currentIsOpen ? 180 : 0 }}
                    transition={{ duration: 0.24, ease: "easeOut" }}
                    className="shrink-0 text-muted"
                  >
                    <AppIcon icon="chevron-down" size={17} />
                  </motion.span>
                </Button>

                <motion.div
                  initial={false}
                  animate={{
                    opacity: currentIsOpen ? 1 : 0,
                    y: currentIsOpen ? 0 : -8,
                  }}
                  transition={{ duration: 0.28, ease: "easeOut" }}
                  className={cn(
                    "absolute inset-x-0 top-[4.25rem] z-10 h-[450px] overflow-hidden rounded-sm border border-border bg-surface p-2 shadow-lg",
                    !currentIsOpen && "pointer-events-none"
                  )}
                >
                  <div className="relative mb-2 w-full">
                    <AppIcon
                      icon="search"
                      size={16}
                      className="pointer-events-none absolute top-1/2 left-3 z-10 -translate-y-1/2 text-muted"
                    />
                    <Input
                      aria-label="Search models"
                      value={currentQuery}
                      readOnly
                      placeholder="Search models"
                      className="w-full pr-4 pl-8"
                    />
                    {currentQuery ? (
                      <span className="pointer-events-none absolute inset-y-0 left-8 flex items-center text-sm">
                        <span className="invisible whitespace-pre">
                          {currentQuery}
                        </span>
                        <motion.span
                          animate={{ opacity: [0.25, 1, 0.25] }}
                          transition={{ duration: 0.8, repeat: Infinity }}
                          className="h-4 w-px bg-foreground"
                        />
                      </span>
                    ) : null}
                  </div>

                  <div className="flex items-center justify-between px-2 py-1 text-[0.68rem] text-muted">
                    <span>
                      {currentQuery
                        ? `Results for “${currentQuery}”`
                        : "All models"}
                    </span>
                    <span>{filteredModels.length} shown</span>
                  </div>

                  <ListBox
                    aria-label="Available models"
                    className="h-[365px] overflow-hidden"
                  >
                    <AnimatePresence initial={false} mode="popLayout">
                      {filteredModels.map((model) => {
                        const isSelected = model.id === currentModelID
                        return (
                          <ListBox.Item
                            key={model.id}
                            id={model.id}
                            textValue={model.name}
                            className={cn(
                              "w-full rounded-sm px-2 py-1.5",
                              isSelected && "bg-accent-soft text-accent"
                            )}
                          >
                            <motion.div
                              layout="position"
                              initial={{ opacity: 0, y: 5 }}
                              animate={{ opacity: 1, y: 0 }}
                              exit={{ opacity: 0, y: -4 }}
                              transition={{ duration: 0.2, ease: "easeOut" }}
                              className="flex w-full min-w-0 items-center gap-3"
                            >
                              <ModelLogo
                                id={model.id}
                                name={model.name}
                                size="sm"
                              />
                              <span className="min-w-0 flex-1">
                                <span className="block truncate text-sm font-medium">
                                  {model.name}
                                </span>
                                <span className="block truncate text-[0.7rem] text-muted">
                                  {model.fit}
                                </span>
                              </span>
                              <AnimatePresence>
                                {isSelected &&
                                (currentIsSelecting || currentIsOpen) ? (
                                  <motion.span
                                    initial={{ scale: 0.7, opacity: 0 }}
                                    animate={{ scale: 1, opacity: 1 }}
                                    exit={{ scale: 0.7, opacity: 0 }}
                                    className="flex size-5 items-center justify-center rounded-full bg-accent text-accent-foreground"
                                  >
                                    <AppIcon icon="check" size={12} />
                                  </motion.span>
                                ) : null}
                              </AnimatePresence>
                            </motion.div>
                          </ListBox.Item>
                        )
                      })}
                    </AnimatePresence>
                  </ListBox>
                </motion.div>
              </div>
            </div>
          </div>

          <div className="flex flex-col justify-center border-t border-border p-6 sm:p-9 lg:border-t-0 lg:border-l lg:p-11">
            <div className="mb-8 flex items-end justify-between gap-4">
              <div>
                <p className="text-xs font-medium tracking-[0.14em] text-muted uppercase">
                  Assigned by job
                </p>
                <h3 className="mt-3 text-2xl leading-tight font-medium tracking-[-0.035em]">
                  Five agents. Five right-sized models.
                </h3>
              </div>
              <span className="hidden text-xs text-muted sm:block">
                Change any time
              </span>
            </div>

            <div className="border-y border-border">
              {agentAssignments.map((assignment, index) => {
                const assignedModelID = currentAssignedModels[assignment.id]
                const model = modelOptions.find(
                  (option) => option.id === assignedModelID
                )
                const isCurrent = assignment.id === currentTargetAgentID
                return (
                  <div
                    key={assignment.name}
                    className={cn(
                      "grid grid-cols-[minmax(0,1fr)_minmax(7.5rem,42%)] items-center gap-4 px-2 py-4 transition-colors duration-300 sm:px-3",
                      index > 0 && "border-t border-border",
                      isCurrent && "bg-accent-soft"
                    )}
                  >
                    <div className="flex min-w-0 items-center gap-3">
                      <span className="flex size-9 shrink-0 items-center justify-center rounded-sm bg-surface-secondary text-muted">
                        <AppIcon icon={assignment.icon} size={18} />
                      </span>
                      <span className="min-w-0">
                        <span className="block truncate text-sm font-medium">
                          {assignment.name}
                        </span>
                        <span className="mt-0.5 block truncate text-xs text-muted">
                          {assignment.job}
                        </span>
                      </span>
                    </div>
                    <div className="flex min-h-7 min-w-0 items-center justify-start">
                      <AnimatePresence initial={false}>
                        {model ? (
                          <motion.div
                            key={model.id}
                            initial={{ opacity: 0, x: 12 }}
                            animate={{ opacity: 1, x: 0 }}
                            exit={{ opacity: 0, x: 8 }}
                            transition={{ duration: 0.34, ease: "easeOut" }}
                            className="flex min-w-0 items-center gap-2"
                          >
                            <ModelLogo
                              id={model.id}
                              name={model.name}
                              size="sm"
                            />
                            <span className="hidden max-w-28 truncate text-xs font-medium sm:block">
                              {model.name}
                            </span>
                          </motion.div>
                        ) : null}
                      </AnimatePresence>
                    </div>
                  </div>
                )
              })}
            </div>

            <p className="mt-7 inline-flex items-center gap-2 text-sm font-medium">
              <AppIcon icon="coins" size={17} className="text-muted" />
              Model prices pass through with 0% markup.
            </p>
          </div>
        </div>
      </div>
    </section>
  )
}
