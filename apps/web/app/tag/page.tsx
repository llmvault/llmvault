import type { Metadata } from "next"
import { TagLandingPage } from "./_components/tag-landing-page"

export const metadata: Metadata = {
  title: "Put Hivy to work in Slack",
  description:
    "Mention @hivy, react to a message, or assign an agent to watch a Slack channel. Hivy does the work and answers in the same thread.",
}

export default function TagPage() {
  return <TagLandingPage />
}
