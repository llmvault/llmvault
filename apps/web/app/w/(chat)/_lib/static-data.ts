import type { CodeLineCommentReference } from "@/app/w/(chat)/_lib/code-line-comments"

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

export type ConversationBlock =
  | AssistantConversationBlock
  | UserConversationBlock
  | ErrorConversationBlock
  | AgentWorkConversationBlock
  | ThinkingConversationBlock
  | ToolConversationBlock
  | ToolChainConversationBlock

export interface AssistantConversationBlock {
  type: "assistant"
  key?: string
  text: string
  streaming?: boolean
}

export interface UserConversationBlock {
  type: "user"
  key?: string
  text: string
  link?: string
  attachments?: MediaAttachment[]
  codeLineComments?: CodeLineCommentReference[]
  author?: Collaborator
}

export interface ErrorConversationBlock {
  type: "error"
  key?: string
  text: string
}

export interface AgentWorkConversationBlock {
  type: "agent_work"
  key?: string
  duration?: string
  blocks: ConversationBlock[]
  active?: boolean
  defaultExpanded?: boolean
}

export interface ThinkingConversationBlock {
  type: "thinking"
  key?: string
  label?: string
  text?: string
  active?: boolean
}

export interface ToolConversationBlock {
  type: "tool"
  key?: string
  label: string
  running?: boolean
  detail?: ToolCallDetail
}

export interface ToolChainConversationBlock {
  type: "tool_chain"
  key?: string
  tools: ToolConversationBlock[]
  running?: boolean
}

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
  actionIcon?: string
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
