import type {
  InputRequestConversationBlock,
  RequestUserInputAnswers,
  RequestUserInputQuestion,
} from "@/app/w/(chat)/_lib/static-data"
import {
  eventTime,
  payloadRecord,
  stringRecordValue,
  stringValue,
  type SessionEventResponse,
} from "@/app/w/(chat)/_lib/session-history-event-utils"

function answeredInputRequestIds(events: SessionEventResponse[]) {
  const ids = new Set<string>()
  for (const event of events) {
    if (event.event_type !== "question_answered") continue
    const id = inputRequestId(payloadRecord(event))
    if (id) ids.add(id)
  }
  return ids
}

export function latestInputRequestBlock(
  events: SessionEventResponse[],
  ignoredIds = new Set<string>()
) {
  let latest: { block: InputRequestConversationBlock; at: number } | undefined
  const answeredIds = answeredInputRequestIds(events)
  for (const event of events) {
    const block = inputRequestBlock(event, answeredIds)
    if (!block || ignoredIds.has(block.questionRequestId)) continue
    const at = eventTime(event)
    if (!latest || at >= latest.at) latest = { block, at }
  }
  return latest?.block
}

export function eventMatchesInputRequest(
  event: SessionEventResponse,
  questionRequestId: string
) {
  if (
    event.event_type !== "question_requested" &&
    event.event_type !== "question_answered"
  ) {
    return false
  }
  return inputRequestId(payloadRecord(event)) === questionRequestId
}

function inputRequestBlock(
  event: SessionEventResponse,
  answeredIds?: Set<string>
): InputRequestConversationBlock | undefined {
  if (event.event_type !== "question_requested") return undefined
  const payload = payloadRecord(event)
  const questionRequestId = inputRequestId(payload)
  const questions = requestInputQuestions(payload)
  if (questionRequestId && answeredIds?.has(questionRequestId)) return undefined
  if (!questionRequestId || questions.length === 0) return undefined
  return {
    type: "input_request",
    key: `input-request:${questionRequestId}`,
    questionRequestId,
    questions,
    answers: requestInputAnswers(payload),
  }
}

function inputRequestId(payload: Record<string, unknown>) {
  return (
    stringValue(payload, "question_request_id") ||
    stringValue(payload, "request_id") ||
    stringValue(payload, "question_id") ||
    stringValue(payload, "id")
  )
}

function requestInputQuestions(
  payload: Record<string, unknown>
): RequestUserInputQuestion[] {
  const questions = payload.questions
  if (!Array.isArray(questions)) return []
  return questions
    .map(requestInputQuestion)
    .filter((question): question is RequestUserInputQuestion =>
      Boolean(question)
    )
}

function requestInputQuestion(value: unknown): RequestUserInputQuestion | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null
  const record = value as Record<string, unknown>
  const id = stringRecordValue(record, "id")
  const header = stringRecordValue(record, "header")
  const question = stringRecordValue(record, "question")
  const options = requestInputOptions(record.options)
  if (!id || !question || options.length === 0) return null
  return { id, header, question, options }
}

function requestInputOptions(value: unknown) {
  if (!Array.isArray(value)) return []
  return value
    .map((option) => {
      if (!option || typeof option !== "object" || Array.isArray(option)) {
        return null
      }
      const record = option as Record<string, unknown>
      const label = stringRecordValue(record, "label")
      const description = stringRecordValue(record, "description")
      return label ? { label, description } : null
    })
    .filter((option): option is { label: string; description: string } =>
      Boolean(option)
    )
}

function requestInputAnswers(
  payload: Record<string, unknown>
): RequestUserInputAnswers | undefined {
  const answers = payload.answers
  if (!answers || typeof answers !== "object" || Array.isArray(answers)) {
    return undefined
  }
  return answers as RequestUserInputAnswers
}
