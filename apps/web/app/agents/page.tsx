import type { Metadata } from "next"
import { AgentsLandingPage } from "./_components/agents-landing-page"

export const metadata: Metadata = {
  title: "Build Hivy agents for repeatable team work",
  description:
    "Install a ready-made specialist or build a Hivy agent with its own instructions, model, tools, knowledge, team access, and sandbox.",
}

export default function AgentsPage() {
  return <AgentsLandingPage />
}
