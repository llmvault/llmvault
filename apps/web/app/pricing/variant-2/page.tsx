import type { Metadata } from "next"
import { PricingPage } from "../_components/pricing-page"

export const metadata: Metadata = {
  title: "Pricing exploration: unlimited",
}

export default function PricingUnlimitedPage() {
  return <PricingPage variant="unlimited" />
}
