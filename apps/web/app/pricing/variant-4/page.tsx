import type { Metadata } from "next"
import { PricingPage } from "../_components/pricing-page"

export const metadata: Metadata = {
  title: "Pricing exploration: manifesto",
}

export default function PricingManifestoPage() {
  return <PricingPage variant="manifesto" />
}
