import type { EmployeeSessionEvent } from "./normalize"

export type Paginated<T> = {
  data?: T[]
  next_cursor?: string | null
  has_more?: boolean
}

export type EmployeeSession = {
  id: string
  runtime_conversation_id?: string
  source?: string
  source_resource_key?: string
  status?: string
  name?: string
  event_count?: number
  created_at?: string
  updated_at?: string
  last_activity_at?: string
}

export type SessionSegment = "web" | "slack"

export type StreamState = {
  text: string
  isStreaming: boolean
  events: EmployeeSessionEvent[]
  error?: string
}

export type SendSessionMessageResponse = {
  created?: boolean
  employee_session_id?: string
  stream_url?: string
  response_stream_url?: string
  runtime_session_id?: string
  source?: string
  source_resource_key?: string
  runtime_conversation_id?: string
}
