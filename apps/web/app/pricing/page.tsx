import type { Metadata } from "next"
import { PricingPage } from "./_components/pricing-page"

export const metadata: Metadata = {
  title: "Pricing",
  description:
    "Add agent credits with a one-time 12% fee. No subscription or seat charge; credits pay for model usage and active sandbox compute.",
}

export default function PricingPageRoute() {
  return <PricingPage />
}
