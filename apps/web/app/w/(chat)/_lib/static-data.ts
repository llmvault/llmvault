export interface MediaAttachment {
  id: string
  filename: string
  kind: "image" | "video"
  url: string
  poster?: string
  duration?: string
}

export interface Collaborator {
  id: string
  name: string
  initials: string
  color: string
  you?: boolean
}

export const currentCollaborator: Collaborator = {
  id: "you",
  name: "You",
  initials: "Y",
  color: "#2563eb",
  you: true,
}

export type ConversationBlock = (
  | { type: "assistant"; text: string; streaming?: boolean }
  | {
      type: "activity"
      label: string
      detail?: { prefix: string; file: string; adds: number; dels: number }
    }
  | {
      type: "user"
      text: string
      link?: string
      attachments?: MediaAttachment[]
      author?: Collaborator
      clientEventID?: string
      clientStatus?: "pending" | "failed"
      clientError?: string
    }
  | { type: "attachments"; items: MediaAttachment[] }
  | { type: "system"; text: string }
  | { type: "error"; text: string }
  | { type: "queued"; author: Collaborator; text: string }
  | { type: "worked"; duration: string; steps: string[] }
  | {
      type: "worklog"
      duration?: string
      startedAt?: number
      blocks: ConversationBlock[]
      active?: boolean
      defaultExpanded?: boolean
    }
  | { type: "working"; duration?: string; by?: Collaborator }
  | {
      type: "tool"
      label: string
      running?: boolean
      detail?: ToolCallDetail
    }
  | {
      type: "thinking"
      label?: string
      duration?: string
      text?: string
      active?: boolean
      defaultExpanded?: boolean
    }
  | {
      type: "edits"
      count: number
      adds: number
      dels: number
      files: { path: string; adds: number; dels: number }[]
      moreFiles?: { path: string; adds: number; dels: number }[]
    }
  | { type: "actions" }
) & { key?: string }

export interface ToolCallDetail {
  tool?: string
  category?:
    | "shell"
    | "web_search"
    | "web_fetch"
    | "file_read"
    | "file_write"
    | "file_edit"
    | "skill_list"
    | "skill"
    | "memory"
    | "session_search"
    | "search"
    | "tool"
  kind: string
  icon?: string
  expandedLabel?: string
  preview?: string
  command?: string
  input?: string
  query?: string
  url?: string
  path?: string
  paths?: string[]
  searchResults?: ToolSearchResult[]
  diff?: string
  bytesWritten?: number
  editsApplied?: number
  output?: string
  error?: string
  exitCode?: number | null
  durationMs?: number
  status?: string
  timedOut?: boolean
  truncated?: boolean
}

export interface ToolSearchResult {
  url: string
  title?: string
}
