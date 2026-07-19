import type { Metadata } from "next"
import { PricingPage } from "../_components/pricing-page"

export const metadata: Metadata = {
  title: "Pricing exploration: receipt",
}

export default function PricingReceiptPage() {
  return <PricingPage variant="receipt" />
}
