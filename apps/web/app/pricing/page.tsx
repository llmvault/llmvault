import type { Metadata } from "next"
import { PricingPage } from "./_components/pricing-page"

export const metadata: Metadata = {
  title: "Pricing",
  description:
    "No subscriptions or model markup. Pay one transparent 12% fee when you add credits.",
}

export default function PricingPageRoute() {
  return <PricingPage variant="plain" />
}
