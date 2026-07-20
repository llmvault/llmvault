import type { Metadata } from "next"
import { KnowledgeLandingPage } from "./_components/knowledge-landing-page"

export const metadata: Metadata = {
  title: "Knowledge base: company memory for Hivy agents",
  description:
    "Connect selected Slack channels, GitHub repositories, Notion pages, Linear teams, and websites. Give Hivy agents cited answers from approved company knowledge.",
}

export default function KnowledgePage() {
  return <KnowledgeLandingPage />
}
