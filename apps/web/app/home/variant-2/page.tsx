import type { Metadata } from "next"
import { CompleteVariantPage } from "../_components/landing-variants"

export const metadata: Metadata = {
  title: "Landing exploration: sticky chapters",
}

export default function HomeVariantTwoPage() {
  return (
    <CompleteVariantPage
      mode="chapters"
      nextHref="/home/variant-3"
      nextLabel="View lifecycle timeline"
    />
  )
}
