import type { Metadata } from "next"
import { AutomationsLandingPage } from "./_components/automations-landing-page"

export const metadata: Metadata = {
  title: "Hivy Automations: run agents from real signals",
  description:
    "Start Hivy agents from pull requests, Slack reactions, schedules, and webhooks, with a complete session for every run.",
}

export default function AutomationsPage() {
  return <AutomationsLandingPage />
}
