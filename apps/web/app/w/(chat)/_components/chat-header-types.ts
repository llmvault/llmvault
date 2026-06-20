import type { Agent } from "@/app/w/(chat)/_lib/agents"

export type ChatHeaderAgent = Pick<Agent, "name" | "icon"> & {
  avatarURL?: string
}
