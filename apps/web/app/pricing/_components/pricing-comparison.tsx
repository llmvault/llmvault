"use client"

import { Link, Tabs } from "@heroui/react"
import { pricingComparisons } from "./pricing-comparison-data"

function ComparisonTable({
  comparison,
}: {
  comparison: (typeof pricingComparisons)[number]
}) {
  return (
    <div className="mt-8 overflow-hidden border-y border-border">
      <div className="overflow-x-auto">
        <table className="w-full min-w-[760px] table-fixed border-collapse text-left">
          <caption className="sr-only">
            Hivy pricing compared with {comparison.name} {comparison.plan}
          </caption>
          <thead>
            <tr className="border-b border-border">
              <th className="w-[22%] px-5 py-5 text-xs font-medium tracking-[0.06em] text-muted uppercase">
                Cost question
              </th>
              <th className="bg-primary/5 w-[39%] border-l border-border px-5 py-5 align-bottom">
                <span className="text-primary block text-xs font-medium tracking-[0.06em] uppercase">
                  Hivy
                </span>
                <span className="mt-2 block text-lg font-medium">
                  No subscription
                </span>
              </th>
              <th className="w-[39%] border-l border-border px-5 py-5 align-bottom">
                <span className="block text-xs font-medium tracking-[0.06em] text-muted uppercase">
                  {comparison.name}
                </span>
                <span className="mt-2 block text-lg font-medium">
                  {comparison.plan}
                </span>
              </th>
            </tr>
          </thead>
          <tbody>
            {comparison.rows.map((row) => (
              <tr
                key={row.label}
                className="border-b border-border last:border-0"
              >
                <th className="px-5 py-6 align-top text-sm font-medium">
                  {row.label}
                </th>
                <td className="bg-primary/5 border-l border-border px-5 py-6 align-top">
                  <p className="text-base font-medium">{row.hivy}</p>
                  <p className="mt-2 max-w-[38ch] text-xs leading-5 text-muted">
                    {row.hivyDetail}
                  </p>
                </td>
                <td className="border-l border-border px-5 py-6 align-top">
                  <p className="text-base font-medium">{row.competitor}</p>
                  <p className="mt-2 max-w-[38ch] text-xs leading-5 text-muted">
                    {row.competitorDetail}
                  </p>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

export function PricingComparisonSection() {
  return (
    <section
      aria-labelledby="pricing-comparison-heading"
      className="mx-auto mt-36 w-[calc(100%-2rem)] max-w-[1300px]"
    >
      <div className="grid gap-6 border-t border-border pt-8 md:grid-cols-[minmax(0,1fr)_minmax(280px,440px)] md:items-end">
        <div>
          <p className="text-xs font-medium tracking-[0.08em] text-muted uppercase">
            Compare the commitment
          </p>
          <h2
            id="pricing-comparison-heading"
            className="mt-4 max-w-[760px] text-[clamp(2.2rem,4vw,4rem)] leading-[0.96] font-medium tracking-[-0.055em]"
          >
            Your business is overpaying for AI work.
          </h2>
        </div>
        <p className="max-w-[56ch] text-sm leading-6 text-muted md:justify-self-end">
          These providers charge recurring platform or seat fees before your
          agents do any work. Compare that monthly commitment with Hivy’s
          usage-based pricing.
        </p>
      </div>

      <Tabs
        variant="primary"
        defaultSelectedKey="claude"
        className="mt-10 w-full"
      >
        <Tabs.ListContainer className="max-w-full overflow-x-auto py-2">
          <Tabs.List
            aria-label="Choose a provider to compare with Hivy"
            className="min-w-max"
          >
            {pricingComparisons.map((comparison) => (
              <Tabs.Tab id={comparison.id} key={comparison.id}>
                {comparison.tabLabel}
                <Tabs.Indicator />
              </Tabs.Tab>
            ))}
          </Tabs.List>
        </Tabs.ListContainer>

        {pricingComparisons.map((comparison) => (
          <Tabs.Panel id={comparison.id} key={comparison.id} className="p-0">
            <div className="mt-8 grid gap-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
              <div>
                <p className="text-2xl font-medium tracking-[-0.035em]">
                  Hivy vs. {comparison.name}
                </p>
                <p className="mt-2 max-w-[68ch] text-sm leading-6 text-muted">
                  {comparison.summary}
                </p>
              </div>
              <p className="text-xs text-muted md:text-right">
                Public prices checked August 10, 2026
              </p>
            </div>

            <ComparisonTable comparison={comparison} />

            <div className="mt-5 flex flex-col gap-3 text-xs text-muted sm:flex-row sm:items-center sm:justify-between">
              <p>
                Products differ in scope. This compares billing mechanics, not
                feature equivalence.
              </p>
              <div className="flex flex-wrap gap-x-4 gap-y-2">
                {comparison.sources.map((source) => (
                  <Link
                    key={source.href}
                    href={source.href}
                    target="_blank"
                    rel="noreferrer"
                    className="text-xs text-foreground underline decoration-border underline-offset-4"
                  >
                    {source.label} ↗
                  </Link>
                ))}
              </div>
            </div>
          </Tabs.Panel>
        ))}
      </Tabs>
    </section>
  )
}
