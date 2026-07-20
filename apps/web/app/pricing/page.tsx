import type { Metadata } from "next"
import { PricingPage } from "./_components/pricing-page"

export const metadata: Metadata = {
  title: "Pricing",
  description:
    "Add agent credits with a one-time 12% fee. No subscription, seat charge, or markup on model and provider costs.",
}

export default function PricingPageRoute() {
  return <PricingPage />
}
