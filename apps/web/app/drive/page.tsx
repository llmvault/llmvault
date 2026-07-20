import type { Metadata } from "next"
import { DriveLandingPage } from "./_components/drive-landing-page"

export const metadata: Metadata = {
  title: "Hivy Drive: one file store for every agent",
  description:
    "Upload files for an agent, keep the files it produces, and search both inputs and outputs across Hivy sessions.",
}

export default function DrivePage() {
  return <DriveLandingPage />
}
