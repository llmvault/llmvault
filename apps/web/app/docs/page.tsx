import Link from "next/link"
import { AppIcon } from "@/components/icon"
import { DOC_SECTIONS } from "./_lib/navigation"

export default function DocsHomePage() {
  return (
    <div className="mx-auto w-full max-w-3xl">
      <header className="max-w-2xl border-b border-border pb-10">
        <p className="text-sm font-medium text-accent">Hivy Documentation</p>
        <h1 className="mt-3 text-4xl leading-tight font-semibold tracking-tight text-foreground sm:text-5xl">
          Run real work with AI teammates.
        </h1>
        <p className="mt-5 max-w-xl text-lg leading-8 text-muted">
          Learn how to organize teams, work with agents, connect company tools,
          and automate recurring work.
        </p>
      </header>

      <div className="mt-12 grid gap-x-12 gap-y-14 md:grid-cols-2">
        {DOC_SECTIONS.map((section) => (
          <section key={section.title}>
            <h2 className="text-sm font-semibold text-foreground">
              {section.title}
            </h2>
            <div className="mt-3 divide-y divide-border border-y border-border">
              {section.pages.map((page) => (
                <Link
                  key={page.slug}
                  href={`/docs/${page.slug}`}
                  className="group flex items-start gap-3 py-4 focus-visible:rounded-md focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus"
                >
                  <span className="min-w-0 flex-1">
                    <span className="block text-sm font-medium text-foreground group-hover:text-accent">
                      {page.title}
                    </span>
                    <span className="mt-1 block text-sm leading-5 text-muted">
                      {page.description}
                    </span>
                  </span>
                  <AppIcon
                    icon="arrow-right"
                    className="mt-0.5 h-4 w-4 shrink-0 text-muted transition-transform duration-200 ease-out group-hover:translate-x-0.5 group-hover:text-foreground"
                  />
                </Link>
              ))}
            </div>
          </section>
        ))}
      </div>
    </div>
  )
}
