import type { Metadata } from "next"
import { AccessControlLandingPage } from "./_components/access-control-landing-page"

export const metadata: Metadata = {
  title: "Hivy access control: govern agents by team",
  description:
    "Group people and agents by team, assign workspace roles, and choose the connections, knowledge sources, and skills each team can use.",
}

export default function AccessControlPage() {
  return <AccessControlLandingPage />
}
