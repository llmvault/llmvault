import type { Metadata } from "next"
import { CompleteVariantPage } from "../_components/landing-variants"

export const metadata: Metadata = {
  title: "Landing exploration: workspace canvas",
}

export default function HomeVariantOnePage() {
  return (
    <CompleteVariantPage
      mode="canvas"
      nextHref="/home/variant-2"
      nextLabel="View sticky chapters"
    />
  )
}
