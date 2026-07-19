import type { Metadata } from "next"
import { CompleteVariantPage } from "../_components/landing-variants"

export const metadata: Metadata = {
  title: "Landing exploration: control room",
}

export default function HomeVariantSixPage() {
  return (
    <CompleteVariantPage
      mode="night"
      nextHref="/home/variant-1"
      nextLabel="Return to workspace canvas"
    />
  )
}
