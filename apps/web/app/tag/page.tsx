import type { Metadata } from "next"
import { TagLandingPage } from "./_components/tag-landing-page"

export const metadata: Metadata = {
  title: "Tag Hivy in Slack",
  description:
    "Assign Hivy agents to Slack channels, tag Hivy in a thread, and keep the work moving without leaving the conversation.",
}

export default function TagPage() {
  return <TagLandingPage />
}
