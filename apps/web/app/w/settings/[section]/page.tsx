import { notFound } from "next/navigation"
import { findNavItem } from "../_components/nav"

export default async function SettingsSectionPage({
  params,
}: {
  params: Promise<{ section: string }>
}) {
  const { section } = await params
  const item = findNavItem(section)

  if (!item) {
    notFound()
  }

  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-2xl font-semibold">{item.label}</h1>
      <div className="rounded-2xl border border-border bg-surface px-5 py-8 text-center">
        <p className="text-sm text-muted">
          This section isn&apos;t part of the static exploration yet — General
          and Usage &amp; billing show the full pattern.
        </p>
      </div>
    </div>
  )
}
