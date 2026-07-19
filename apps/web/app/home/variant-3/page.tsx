import type { Metadata } from "next"
import { CompleteVariantPage } from "../_components/landing-variants"

export const metadata: Metadata = {
  title: "Landing exploration: lifecycle timeline",
}

export default function HomeVariantThreePage() {
  return (
    <CompleteVariantPage
      mode="timeline"
      nextHref="/home/variant-4"
      nextLabel="View editorial ledger"
    />
  )
}
