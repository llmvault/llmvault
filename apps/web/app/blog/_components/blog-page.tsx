"use client"

import { type ReactNode } from "react"
import { Button, Link, Tabs } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { LandingHeader } from "../../home/_components/landing-header"
import { LandingFooter } from "../../home/_components/landing-shared"
import {
  blogCategories,
  blogPosts,
  type BlogCategory,
  type BlogPost,
} from "./blog-content"

function articleHref(post: BlogPost) {
  return `/blog/${post.slug}`
}

function StoryLink({
  post,
  children,
  className = "",
}: {
  post: BlogPost
  children: ReactNode
  className?: string
}) {
  return (
    <Link
      href={articleHref(post)}
      aria-label={`Read ${post.title}`}
      className={`group text-foreground ${className}`}
    >
      {children}
    </Link>
  )
}

function CoverFrame({
  post,
  children,
  className = "",
}: {
  post: BlogPost
  children: ReactNode
  className?: string
}) {
  return (
    <div
      className={`relative overflow-hidden rounded-sm border border-border bg-surface-secondary ${className}`}
    >
      <div className="absolute inset-x-5 top-4 z-10 flex items-center justify-between font-mono text-[0.62rem] tracking-[0.14em] text-muted uppercase">
        <span>Hivy journal</span>
        <span>{post.issue.padStart(3, "0")}</span>
      </div>
      {children}
      <div className="absolute right-5 bottom-4 left-5 z-10 flex items-center justify-between font-mono text-[0.62rem] tracking-[0.1em] text-muted uppercase">
        <span>{post.category}</span>
        <span>{post.readTime}</span>
      </div>
    </div>
  )
}

function ModelRouteCover({ post }: { post: BlogPost }) {
  const nodes = [
    ["message-square", "Request"],
    ["brain", "Model"],
    ["bot", "Agent"],
    ["check", "Result"],
  ] as const

  return (
    <CoverFrame post={post} className="aspect-[16/10]">
      <div className="absolute inset-x-[11%] top-1/2 h-px bg-border" />
      <div className="absolute inset-x-[11%] top-1/2 flex -translate-y-1/2 items-center justify-between">
        {nodes.map(([icon, label], index) => (
          <div key={label} className="flex flex-col items-center gap-3">
            <span
              className={`flex size-11 items-center justify-center rounded-sm border ${index === 1 ? "border-accent bg-accent text-accent-foreground" : "border-border bg-surface text-muted"}`}
            >
              <AppIcon icon={icon} size={18} />
            </span>
            <span className="hidden font-mono text-[0.6rem] text-muted uppercase sm:block">
              {label}
            </span>
          </div>
        ))}
      </div>
    </CoverFrame>
  )
}

function ThreadCover({ post }: { post: BlogPost }) {
  return (
    <CoverFrame post={post} className="aspect-[16/10]">
      <div className="absolute inset-x-[9%] top-[28%] space-y-3">
        {[
          ["message-square", "Request from #support", "w-[82%]"],
          ["bot", "Assigned to Support agent", "ml-auto w-[74%]"],
          ["check", "Answer returned to the thread", "w-[88%]"],
        ].map(([icon, label, width], index) => (
          <div
            key={label}
            className={`flex items-center gap-3 rounded-sm border border-border bg-surface px-3 py-2.5 ${width}`}
          >
            <span
              className={`flex size-7 shrink-0 items-center justify-center rounded-sm ${index === 1 ? "bg-accent text-accent-foreground" : "bg-surface-secondary text-muted"}`}
            >
              <AppIcon icon={icon} size={14} />
            </span>
            <span className="truncate text-xs text-muted">{label}</span>
          </div>
        ))}
      </div>
    </CoverFrame>
  )
}

function AccessCover({ post }: { post: BlogPost }) {
  return (
    <CoverFrame post={post} className="aspect-[16/10]">
      <div className="absolute inset-0 flex items-center justify-center">
        <div className="relative flex size-[48%] items-center justify-center rounded-full border border-border">
          <div className="flex size-[68%] items-center justify-center rounded-full border border-border bg-surface">
            <span className="flex size-12 items-center justify-center rounded-sm bg-accent text-accent-foreground">
              <AppIcon icon="shield-check" size={22} />
            </span>
          </div>
          {["plug", "database", "sparkles"].map((icon, index) => (
            <span
              key={icon}
              className="absolute flex size-8 items-center justify-center rounded-sm border border-border bg-surface text-muted"
              style={{
                top: index === 0 ? "-12%" : index === 1 ? "74%" : "12%",
                left: index === 0 ? "12%" : index === 1 ? "-8%" : "88%",
              }}
            >
              <AppIcon icon={icon} size={15} />
            </span>
          ))}
        </div>
      </div>
    </CoverFrame>
  )
}

function KnowledgeCover({ post }: { post: BlogPost }) {
  return (
    <CoverFrame post={post} className="aspect-[16/10]">
      <div className="absolute inset-x-[11%] top-[25%] grid grid-cols-6 gap-2 sm:gap-3">
        {Array.from({ length: 24 }, (_, index) => (
          <span
            key={index}
            className={`aspect-square rounded-[2px] border ${[4, 9, 10, 15, 20].includes(index) ? "border-accent bg-accent" : "border-border bg-surface"}`}
          />
        ))}
      </div>
    </CoverFrame>
  )
}

function WorkflowCover({ post }: { post: BlogPost }) {
  return (
    <CoverFrame post={post} className="aspect-[16/10]">
      <div className="absolute inset-x-[11%] top-1/2 h-px bg-border" />
      <div className="absolute inset-x-[11%] top-1/2 flex -translate-y-1/2 items-center justify-between">
        {[
          ["plug", "Connected event"],
          ["calendar", "Schedule"],
          ["globe", "Webhook"],
        ].map(([icon, label], index) => (
          <div key={label} className="flex flex-col items-center gap-3">
            <span
              className={`flex size-12 items-center justify-center rounded-sm border ${index === 1 ? "border-accent bg-accent text-accent-foreground" : "border-border bg-surface text-muted"}`}
            >
              <AppIcon icon={icon} size={19} />
            </span>
            <span className="hidden text-[0.65rem] text-muted sm:block">
              {label}
            </span>
          </div>
        ))}
      </div>
    </CoverFrame>
  )
}

function StoryCover({ post, index = 0 }: { post: BlogPost; index?: number }) {
  const mode = index % 5
  if (mode === 1) return <ThreadCover post={post} />
  if (mode === 2) return <AccessCover post={post} />
  if (mode === 3) return <KnowledgeCover post={post} />
  if (mode === 4) return <WorkflowCover post={post} />
  return <ModelRouteCover post={post} />
}

function PostMetadata({ post }: { post: BlogPost }) {
  return (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[0.68rem] tracking-[0.06em] uppercase">
      <span className="text-accent">{post.category}</span>
      <span className="text-muted" aria-hidden="true">
        ·
      </span>
      <span className="text-muted">{post.date}</span>
      <span className="text-muted" aria-hidden="true">
        ·
      </span>
      <span className="text-muted">{post.readTime}</span>
    </div>
  )
}

function PostCollection({ category }: { category: BlogCategory }) {
  const posts = blogPosts.filter(
    (post) => category === "All" || post.category === category
  )
  const [featured, ...latest] = posts

  if (!featured) return null

  return (
    <div className="mt-10">
      <StoryLink
        post={featured}
        className="grid min-w-0 gap-8 lg:grid-cols-[1.22fr_0.78fr] lg:items-center lg:gap-12"
      >
        <div className="min-w-0 transition-transform duration-300 ease-out group-hover:-translate-y-1">
          <ModelRouteCover post={featured} />
        </div>
        <div className="min-w-0 py-2 lg:py-8">
          <PostMetadata post={featured} />
          <h2 className="mt-5 max-w-[560px] text-[clamp(2rem,3.4vw,3.25rem)] leading-[0.98] font-medium tracking-[-0.05em] group-hover:text-accent">
            {featured.title}
          </h2>
          <p className="mt-5 max-w-[62ch] text-sm leading-6 text-muted">
            {featured.excerpt}
          </p>
          <span className="mt-7 inline-flex items-center gap-2 text-sm font-medium">
            Read post
            <AppIcon
              icon="arrow-right"
              size={15}
              className="transition-transform duration-200 group-hover:translate-x-1"
            />
          </span>
        </div>
      </StoryLink>

      {latest.length > 0 ? (
        <div className="mt-16 border-t border-border pt-8">
          <div className="flex items-center justify-between gap-4">
            <h2 className="text-[0.72rem] font-medium tracking-[0.14em] text-muted uppercase">
              Latest posts
            </h2>
            <span className="text-xs text-muted">
              {latest.length} {latest.length === 1 ? "story" : "stories"}
            </span>
          </div>
          <div className="mt-7 grid gap-x-7 gap-y-14 md:grid-cols-2 lg:grid-cols-3">
            {latest.map((post, index) => (
              <StoryLink
                key={post.slug}
                post={post}
                className="min-w-0 flex-col"
              >
                <div className="w-full transition-transform duration-300 ease-out group-hover:-translate-y-1">
                  <StoryCover post={post} index={index + 1} />
                </div>
                <div className="mt-5 min-w-0">
                  <PostMetadata post={post} />
                  <h3 className="mt-3 text-[1.15rem] leading-snug font-medium tracking-[-0.025em] group-hover:text-accent">
                    {post.title}
                  </h3>
                  <p className="mt-3 text-sm leading-6 text-muted">
                    {post.excerpt}
                  </p>
                </div>
              </StoryLink>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  )
}

function ClosingCta() {
  return (
    <section className="mx-auto mt-20 w-[calc(100%-2rem)] max-w-[1300px] py-14 text-center md:mt-24 md:py-20">
      <p className="text-[0.7rem] font-medium tracking-[0.14em] text-muted uppercase">
        Put the ideas to work
      </p>
      <h2 className="mx-auto mt-5 max-w-[800px] text-[clamp(2.4rem,5vw,4.75rem)] leading-[0.94] font-medium tracking-[-0.06em]">
        Give your team agents that can finish the job.
      </h2>
      <Link href="/auth/signup" className="mt-8 inline-flex">
        <Button variant="primary">Start for free</Button>
      </Link>
    </section>
  )
}

export function BlogPage() {
  return (
    <main className="marketing-link-scope min-h-screen bg-background text-foreground">
      <LandingHeader />
      <section className="mx-auto w-[calc(100%-2rem)] max-w-[1300px] pt-24 md:pt-32">
        <div>
          <h1 className="text-[clamp(3rem,6vw,5.5rem)] leading-[0.88] font-medium tracking-[-0.07em]">
            Blog
          </h1>
          <p className="mt-5 max-w-[700px] text-base leading-7 text-muted md:text-lg">
            Practical notes on building, operating, and improving AI agents for
            real teams.
          </p>
        </div>

        <Tabs
          variant="primary"
          defaultSelectedKey="All"
          className="mt-10 w-full min-w-0"
        >
          <Tabs.ListContainer className="w-full max-w-full overflow-x-auto">
            <Tabs.List
              aria-label="Browse Hivy blog posts by category"
              className="w-fit min-w-[620px]"
            >
              {blogCategories.map((category) => (
                <Tabs.Tab id={category} key={category}>
                  {category === "All" ? "All posts" : category}
                  <Tabs.Indicator />
                </Tabs.Tab>
              ))}
            </Tabs.List>
          </Tabs.ListContainer>
          {blogCategories.map((category) => (
            <Tabs.Panel id={category} key={category} className="min-w-0 p-0">
              <PostCollection category={category} />
            </Tabs.Panel>
          ))}
        </Tabs>
      </section>
      <ClosingCta />
      <LandingFooter />
    </main>
  )
}
