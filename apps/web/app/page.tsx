import { Button, Link } from "@heroui/react"
import { Icon } from "@iconify/react"
import Image from "next/image"
import { WatchDemo } from "@/components/watch-demo"

const GITHUB_URL = "https://github.com/usehivy/hivy"
// TODO: replace with the real waitlist form URL (Tally/Typeform/etc.)
const WAITLIST_URL = "#"

const integrations = [
  { icon: "mdi:github", name: "GitHub" },
  { icon: "mdi:slack", name: "Slack" },
  { icon: "simple-icons:linear", name: "Linear" },
  { icon: "simple-icons:notion", name: "Notion" },
  { icon: "simple-icons:vercel", name: "Vercel" },
  { icon: "simple-icons:railway", name: "Railway" },
  { icon: "simple-icons:postgresql", name: "Postgres" },
  { icon: "simple-icons:redis", name: "Redis" },
]

export default function RootPage() {
  return (
    <main className="relative flex h-screen flex-col overflow-hidden">
      {/* Soft background graphic */}
      <div className="pointer-events-none absolute inset-0 -z-10">
        <Image
          src="/hero-mountains-background.png"
          alt=""
          fill
          priority
          sizes="100vw"
          className="object-cover opacity-30"
        />
        <div className="absolute inset-0 bg-gradient-to-b from-background/70 via-background/85 to-background" />
      </div>

      {/* Top bar */}
      <header className="flex items-center justify-between px-5 py-5 sm:px-8">
        <Image
          src="/logomark-no-background.png"
          alt="Hivy"
          width={36}
          height={36}
          priority
          className="object-contain"
        />
        <Link
          href={GITHUB_URL}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-2 rounded-full border border-border bg-surface/70 px-4 py-2 text-sm font-medium text-foreground backdrop-blur transition-colors hover:bg-surface-secondary"
        >
          <Icon icon="mdi:github" className="h-4 w-4" />
          <span className="hidden sm:inline">Star on GitHub</span>
        </Link>
      </header>

      {/* Left-aligned hero, flush to the browser edge, anchored to the bottom */}
      <div className="flex w-full flex-1 flex-col items-start justify-end px-5 pb-10 text-left sm:px-8 sm:pb-14">
        {/* Large graphic */}
        <Image
          src="/logomark-no-background.png"
          alt="Hivy logomark"
          width={160}
          height={160}
          priority
          className="h-24 w-24 object-contain drop-shadow-xl sm:h-32 sm:w-32"
        />

        <h1 className="mt-8 max-w-4xl text-5xl leading-[0.95] font-medium tracking-tight text-balance text-foreground sm:text-7xl">
          Productive AI agents for your entire team.
        </h1>

        <p className="mt-6 max-w-2xl text-lg leading-8 text-balance text-foreground/70 sm:text-xl">
          Hivy gives every team AI teammates that work in real cloud workspaces —
          using your tools, your data, and your context to do real work, not just
          chat.
        </p>

        <div className="mt-9 flex flex-col items-stretch gap-3 sm:flex-row sm:items-center">
          <Link href={WAITLIST_URL}>
            <Button size="lg">Join the waitlist</Button>
          </Link>
          <WatchDemo />
        </div>

        {/* Integrations */}
        <div className="mt-14">
          <p className="text-xs font-medium tracking-wide text-muted uppercase">
            Works with the tools your team already uses
          </p>
          <div className="mt-5 flex flex-wrap items-center gap-x-8 gap-y-4">
            {integrations.map((tool) => (
              <span
                key={tool.name}
                className="inline-flex items-center gap-2 text-foreground/55 transition-colors hover:text-foreground"
              >
                <Icon icon={tool.icon} className="h-6 w-6" />
                <span className="text-sm font-medium">{tool.name}</span>
              </span>
            ))}
          </div>
        </div>
      </div>
    </main>
  )
}
