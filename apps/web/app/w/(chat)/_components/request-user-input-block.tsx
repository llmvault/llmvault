"use client"

import { useEffect, useMemo, useState } from "react"
import { Icon } from "@iconify/react"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import type {
  InputRequestConversationBlock,
  RequestUserInputAnswers,
  RequestUserInputQuestion,
} from "@/app/w/(chat)/_lib/static-data"

const OTHER_LABEL = "Other"

type AnswerDraft = Record<string, { label: string; other?: string }>

export function RequestUserInputBlock({
  block,
  sessionId,
  agentName,
  onSubmitted,
  inline = false,
}: {
  block: InputRequestConversationBlock
  sessionId?: string
  agentName?: string
  onSubmitted?: (questionRequestId: string) => void
  inline?: boolean
}) {
  const initialAnswers = useMemo(
    () => initialAnswerDraft(block.questions, block.answers),
    [block.answers, block.questions]
  )
  const [answers, setAnswers] = useState<AnswerDraft>(initialAnswers)
  const [step, setStep] = useState(0)
  const [submitted, setSubmitted] = useState(Boolean(block.answers))
  const [error, setError] = useState("")
  const [pendingAction, setPendingAction] = useState<
    "dismiss" | "submit" | null
  >(null)
  const submitInputResponse = $api.useMutation(
    "post",
    "/v1/sessions/{id}/input-responses"
  )
  const question = block.questions[step]
  const isLastStep = step >= block.questions.length - 1
  const payload = buildQuestionAnswerPayload(block.questions, answers)
  const currentAnswerValid = question
    ? answerIsValid(answers[question.id])
    : false
  const disabled = !sessionId || submitInputResponse.isPending
  const agentLabel = agentName?.trim() || "the agent"

  useEffect(() => {
    setAnswers(initialAnswers)
    setStep(0)
    setSubmitted(Boolean(block.answers))
    setError("")
    setPendingAction(null)
  }, [block.answers, block.questionRequestId, initialAnswers])

  if (!question || submitted) return null

  const select = (questionId: string, label: string, other?: string) => {
    setError("")
    setAnswers((current) => ({
      ...current,
      [questionId]: { label, other },
    }))
  }

  const next = () => {
    if (!currentAnswerValid) {
      setError("Choose an option before continuing.")
      return
    }
    setError("")
    setStep((current) => Math.min(current + 1, block.questions.length - 1))
  }

  const submitPayload = async (
    answersPayload: NonNullable<ReturnType<typeof buildQuestionAnswerPayload>>,
    action: "dismiss" | "submit"
  ) => {
    if (!sessionId) return
    setError("")
    setPendingAction(action)
    try {
      await submitInputResponse.mutateAsync({
        params: { path: { id: sessionId } },
        body: {
          question_request_id: block.questionRequestId,
          answers: answersPayload.answers,
        },
      })
      setSubmitted(true)
      onSubmitted?.(block.questionRequestId)
    } catch (cause) {
      setError(
        extractErrorMessage(
          cause,
          action === "dismiss"
            ? "Could not dismiss request"
            : "Could not submit answer"
        )
      )
    } finally {
      setPendingAction(null)
    }
  }

  const dismiss = () =>
    submitPayload(buildDismissalAnswerPayload(block.questions), "dismiss")

  const submit = async () => {
    if (!sessionId || !payload) {
      setError("Answer every question before submitting.")
      return
    }
    await submitPayload(payload, "submit")
  }

  const content = (
    <section
      className={
        inline
          ? "pointer-events-auto w-full"
          : "pointer-events-auto mx-auto w-full max-w-3xl px-4"
      }
      onKeyDown={(event) => {
        if (event.key === "Escape") void dismiss()
      }}
    >
      <div className="rounded-2xl border border-border bg-surface px-3 py-3 shadow-lg">
        <div className="flex items-start gap-3">
          <div className="min-w-0 flex-1">
            {block.questions.length > 1 ? (
              <div className="mb-1 text-xs font-medium text-muted">
                Question {step + 1} / {block.questions.length}
              </div>
            ) : null}
            <h3 className="text-sm leading-5 font-medium text-foreground">
              {question.question}
            </h3>
          </div>
        </div>

        <div className="mt-2 flex flex-col gap-1">
          {question.options.map((option, index) => (
            <OptionRow
              key={option.label}
              index={index + 1}
              label={option.label}
              description={option.description}
              selected={answers[question.id]?.label === option.label}
              disabled={submitInputResponse.isPending}
              onClick={() => select(question.id, option.label)}
            />
          ))}
          <OtherRow
            index={question.options.length + 1}
            agentName={agentLabel}
            value={
              answers[question.id]?.label === OTHER_LABEL
                ? (answers[question.id]?.other ?? "")
                : ""
            }
            selected={answers[question.id]?.label === OTHER_LABEL}
            disabled={submitInputResponse.isPending}
            onChange={(value) => select(question.id, OTHER_LABEL, value)}
          />
        </div>

        <div className="mt-3 flex items-center justify-between gap-3">
          <p className="min-w-0 flex-1 text-xs text-danger">{error}</p>
          <div className="flex shrink-0 items-center gap-2">
            {step > 0 ? (
              <button
                type="button"
                disabled={submitInputResponse.isPending}
                onClick={() => {
                  setError("")
                  setStep((current) => Math.max(0, current - 1))
                }}
                className="inline-flex h-8 items-center gap-1 rounded-lg px-2.5 text-xs text-muted transition-colors hover:bg-default hover:text-foreground focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-warning disabled:cursor-not-allowed disabled:opacity-50"
              >
                <Icon icon="lucide:arrow-left" className="h-3.5 w-3.5" />
                Back
              </button>
            ) : null}
            <button
              type="button"
              disabled={submitInputResponse.isPending}
              onClick={() => void dismiss()}
              className="inline-flex h-8 items-center gap-1 rounded-lg px-2.5 text-xs text-muted transition-colors hover:bg-default hover:text-foreground focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-warning disabled:cursor-not-allowed disabled:opacity-50"
            >
              {pendingAction === "dismiss" ? (
                <Icon
                  icon="lucide:loader-circle"
                  className="h-3.5 w-3.5 animate-spin"
                />
              ) : null}
              Dismiss
              <kbd className="rounded bg-default px-1.5 py-0.5 text-[11px] text-foreground">
                ESC
              </kbd>
            </button>
            {isLastStep ? (
              <button
                type="button"
                disabled={disabled}
                onClick={() => void submit()}
                className="inline-flex h-8 items-center gap-1.5 rounded-lg bg-accent px-3 text-xs font-medium text-accent-foreground transition-colors hover:bg-accent/90 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-warning disabled:cursor-not-allowed disabled:opacity-50"
              >
                {pendingAction === "submit" ? (
                  <Icon
                    icon="lucide:loader-circle"
                    className="h-3.5 w-3.5 animate-spin"
                  />
                ) : null}
                Submit
                <Icon icon="lucide:corner-down-left" className="h-3.5 w-3.5" />
              </button>
            ) : (
              <button
                type="button"
                disabled={disabled}
                onClick={next}
                className="inline-flex h-8 items-center gap-1.5 rounded-lg bg-foreground px-3 text-xs font-medium text-background transition-colors hover:bg-foreground/90 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-warning disabled:cursor-not-allowed disabled:opacity-50"
              >
                Next
                <Icon icon="lucide:arrow-right" className="h-3.5 w-3.5" />
              </button>
            )}
          </div>
        </div>
      </div>
    </section>
  )

  if (inline) return content

  return (
    <div className="pointer-events-none absolute inset-x-0 bottom-full z-30 pb-2">
      {content}
    </div>
  )
}

function OptionRow({
  index,
  label,
  description,
  selected,
  disabled,
  onClick,
}: {
  index: number
  label: string
  description: string
  selected: boolean
  disabled: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={`grid min-h-8 grid-cols-[auto_1fr_auto] items-center gap-2 rounded-lg px-2 text-left transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-warning ${
        selected
          ? "bg-default text-foreground"
          : "text-foreground hover:bg-default/60"
      } disabled:cursor-not-allowed disabled:opacity-60`}
    >
      <NumberBadge index={index} selected={selected} />
      <span className="min-w-0 truncate text-sm">{label}</span>
      {description ? (
        <span title={description} className="text-muted">
          <Icon icon="lucide:info" className="h-3.5 w-3.5" />
        </span>
      ) : null}
    </button>
  )
}

function OtherRow({
  index,
  agentName,
  value,
  selected,
  disabled,
  onChange,
}: {
  index: number
  agentName: string
  value: string
  selected: boolean
  disabled: boolean
  onChange: (value: string) => void
}) {
  return (
    <label
      className={`grid min-h-8 grid-cols-[auto_1fr] items-center gap-2 rounded-lg px-2 transition-colors ${
        selected ? "bg-default" : "hover:bg-default/60"
      }`}
    >
      <NumberBadge index={index} selected={selected} pencil={selected} />
      <input
        disabled={disabled}
        value={value}
        onFocus={() => {
          if (!selected) onChange("")
        }}
        onChange={(event) => onChange(event.target.value)}
        placeholder={`No, tell ${agentName} what to do differently`}
        className="min-w-0 bg-transparent text-sm text-foreground outline-none placeholder:text-muted disabled:cursor-not-allowed"
      />
    </label>
  )
}

function NumberBadge({
  index,
  selected,
  pencil = false,
}: {
  index: number
  selected: boolean
  pencil?: boolean
}) {
  return (
    <span
      className={`flex h-5 w-5 items-center justify-center rounded-full text-xs font-medium ${
        selected
          ? "bg-foreground text-background"
          : "border border-border bg-background text-muted"
      }`}
    >
      {pencil ? <Icon icon="lucide:pencil" className="h-3 w-3" /> : index}
    </span>
  )
}

function answerIsValid(answer: { label: string; other?: string } | undefined) {
  if (!answer?.label) return false
  if (answer.label !== OTHER_LABEL) return true
  return Boolean(answer.other?.trim())
}

export function initialAnswerDraft(
  questions: RequestUserInputQuestion[],
  answers?: RequestUserInputAnswers
): AnswerDraft {
  return Object.fromEntries(
    questions.map((question) => {
      const existing = answers?.[question.id]
      const selected = existing?.answers[0] ?? question.options[0]?.label ?? ""
      return [
        question.id,
        {
          label: selected,
          other: existing?.other,
        },
      ]
    })
  )
}

export function buildQuestionAnswerPayload(
  questions: RequestUserInputQuestion[],
  answers: AnswerDraft
) {
  const out: RequestUserInputAnswers = {}
  for (const question of questions) {
    const answer = answers[question.id]
    if (!answerIsValid(answer)) return null
    if (answer.label === OTHER_LABEL) {
      out[question.id] = {
        answers: [OTHER_LABEL],
        other: answer.other?.trim(),
      }
      continue
    }
    out[question.id] = { answers: [answer.label] }
  }
  return { answers: out }
}

export function buildDismissalAnswerPayload(
  questions: RequestUserInputQuestion[]
) {
  const out: RequestUserInputAnswers = {}
  for (const question of questions) {
    out[question.id] = {
      answers: [OTHER_LABEL],
      other: "Dismissed by user.",
    }
  }
  return { answers: out }
}
