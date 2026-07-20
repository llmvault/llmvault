import type { Metadata } from "next"
import { SheetsLandingPage } from "./_components/sheets-landing-page"

export const metadata: Metadata = {
  title: "Hivy Sheets: a database agents can read and write",
  description:
    "Store structured team records in one Hivy Sheet that people and agents can query, update, and return to across sessions.",
}

export default function SheetsPage() {
  return <SheetsLandingPage />
}
