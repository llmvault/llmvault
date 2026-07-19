import type { Metadata } from "next"
import { CompleteVariantPage } from "../_components/landing-variants"

export const metadata: Metadata = {
  title: "Landing exploration: workspace layers",
}

export default function HomeVariantFivePage() {
  return (
    <CompleteVariantPage
      mode="bands"
      nextHref="/home/variant-6"
      nextLabel="View control room"
    />
  )
}
