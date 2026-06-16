import { notFound } from "next/navigation"
import { Button, Link } from "@heroui/react"
import { Icon } from "@iconify/react"
import { findPluginBySlug } from "@/app/w/(chat)/plugins/_data"

export default async function PluginDetailPage({
  params,
}: {
  params: Promise<{ slug: string }>
}) {
  const { slug } = await params
  const plugin = findPluginBySlug(slug)

  if (!plugin) {
    notFound()
  }

  const displayCategory = plugin.detailCategory ?? plugin.category
  const examples = plugin.examples ?? []
  const skills = plugin.skills ?? []
  const developer = plugin.developer ?? "Hivy"
  const capabilities = plugin.capabilities ?? ["Read"]
  const links = plugin.links ?? {}
  const longDescription = plugin.longDescription ?? plugin.description

  return (
    <div className="h-full overflow-y-auto bg-background text-foreground">
      <div className="mx-auto w-full max-w-2xl px-6 py-12">
        <div className="flex flex-col gap-10">
          <header className="flex items-start justify-between gap-4">
            <div className="flex items-center gap-4">
              <div
                className="flex h-16 w-16 shrink-0 items-center justify-center rounded-2xl text-white"
                style={{ backgroundColor: plugin.iconColor }}
              >
                <AppIcon icon={plugin.icon} color="#FFFFFF" size={32} />
              </div>
              <div>
                <h1 className="text-3xl font-semibold text-foreground">
                  {plugin.name}
                </h1>
                <p className="text-lg text-muted-foreground">
                  {plugin.description}
                </p>
              </div>
            </div>
            <Button className="shrink-0 rounded-full bg-foreground text-background hover:bg-foreground/90">
              Add
            </Button>
          </header>

          {examples.length > 0 ? (
            <section className="flex flex-col gap-4">
              <h2 className="text-xl font-semibold text-foreground">
                Examples
              </h2>
              <div className="flex flex-col divide-y divide-border rounded-xl border border-border bg-card">
                {examples.map((example, index) => (
                  <button
                    key={index}
                    type="button"
                    className="group flex items-center justify-between gap-4 p-4 text-left transition-colors hover:bg-muted/20"
                  >
                    <div className="flex items-center gap-3">
                      <div
                        className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-white"
                        style={{ backgroundColor: plugin.iconColor }}
                      >
                        <AppIcon icon={plugin.icon} color="#FFFFFF" size={16} />
                      </div>
                      <span className="text-sm text-foreground">{example}</span>
                    </div>
                    <Icon
                      icon="lucide:arrow-right"
                      className="h-4 w-4 shrink-0 text-muted-foreground transition-colors group-hover:text-foreground"
                    />
                  </button>
                ))}
              </div>
            </section>
          ) : null}

          {skills.length > 0 ? <ContainsSection skills={skills} /> : null}

          <section className="flex flex-col gap-4">
            <h2 className="text-xl font-semibold text-foreground">
              Developed by {developer}
            </h2>
            <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
              <div>
                <p className="text-sm text-muted-foreground">Category</p>
                <p className="text-sm font-medium text-foreground">
                  {displayCategory}
                </p>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Links</p>
                <div className="flex flex-col items-start gap-1">
                  {links.website ? (
                    <Link
                      href={links.website}
                      className="inline-flex items-center gap-1 text-sm"
                    >
                      Website
                      <Icon
                        icon="lucide:external-link"
                        className="h-3.5 w-3.5"
                      />
                    </Link>
                  ) : null}
                  {links.privacy ? (
                    <Link
                      href={links.privacy}
                      className="inline-flex items-center gap-1 text-sm"
                    >
                      Privacy Policy
                      <Icon
                        icon="lucide:external-link"
                        className="h-3.5 w-3.5"
                      />
                    </Link>
                  ) : null}
                  {links.terms ? (
                    <Link
                      href={links.terms}
                      className="inline-flex items-center gap-1 text-sm"
                    >
                      Terms of Service
                      <Icon
                        icon="lucide:external-link"
                        className="h-3.5 w-3.5"
                      />
                    </Link>
                  ) : null}
                </div>
              </div>
              <div>
                <p className="text-sm text-muted-foreground">Capabilities</p>
                <p className="text-sm font-medium text-foreground">
                  {capabilities.join(", ")}
                </p>
              </div>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Description</p>
              <p className="mt-1 text-sm leading-relaxed text-foreground">
                {longDescription}
              </p>
            </div>
          </section>
        </div>
      </div>
    </div>
  )
}

function ContainsSection({
  skills,
}: {
  skills: Array<{ name: string; description: string }>
}) {
  const previewCount = Math.min(5, skills.length)
  const hasMore = skills.length > previewCount

  return (
    <section className="flex flex-col gap-4">
      <div>
        <h2 className="text-xl font-semibold text-foreground">Contains</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Skills {skills.length}
        </p>
      </div>
      <div className="flex flex-col gap-4">
        {skills.slice(0, previewCount).map((skill) => (
          <div
            key={skill.name}
            className="grid grid-cols-1 gap-1 sm:grid-cols-[1fr_2fr]"
          >
            <span className="text-sm font-medium text-foreground">
              {skill.name}
            </span>
            <span className="text-sm text-muted-foreground">
              {skill.description}
            </span>
          </div>
        ))}
      </div>
      {hasMore ? (
        <button
          type="button"
          className="text-left text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          Show {skills.length - previewCount} more
        </button>
      ) : null}
    </section>
  )
}

function AppIcon({
  icon,
  color,
  size = 20,
}: {
  icon: string
  color: string
  size?: number
}) {
  return (
    <Icon
      icon={icon}
      className="shrink-0"
      style={{ color, width: size, height: size }}
    />
  )
}
