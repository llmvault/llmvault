import type { Metadata } from "next"
import { CompleteVariantPage } from "../_components/landing-variants"

export const metadata: Metadata = {
  title: "Landing exploration: editorial ledger",
}

export default function HomeVariantFourPage() {
  return (
    <CompleteVariantPage
      mode="ledger"
      nextHref="/home/variant-5"
      nextLabel="View workspace layers"
    />
  )
}
