import type { Metadata } from "next"
import { notFound } from "next/navigation"
import { Button, Link } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { LandingHeader } from "../../home/_components/landing-header"
import { LandingFooter } from "../../home/_components/landing-shared"
import { getBlogArticleContent } from "../_components/blog-article-content"
import {
  blogPosts,
  getBlogPost,
  type BlogPost,
} from "../_components/blog-content"

type BlogArticlePageProps = {
  params: Promise<{ slug: string }>
}

export function generateStaticParams() {
  return blogPosts.map((post) => ({ slug: post.slug }))
}

export async function generateMetadata({
  params,
}: BlogArticlePageProps): Promise<Metadata> {
  const { slug } = await params
  const post = getBlogPost(slug)
  if (!post) return {}

  return {
    title: post.title,
    description: post.excerpt,
  }
}

function ArticleArtwork({ post }: { post: BlogPost }) {
  const steps = [
    ["message-square", "Request"],
    [post.icon, post.category],
    ["bot", "Agent"],
    ["check", "Result"],
  ] as const

  return (
    <div className="relative mt-12 aspect-[16/8] overflow-hidden rounded-sm border border-border bg-surface-secondary sm:mt-16">
      <div className="absolute inset-x-5 top-4 flex items-center justify-between font-mono text-[0.62rem] tracking-[0.14em] text-muted uppercase sm:inset-x-7 sm:top-6">
        <span>Hivy journal</span>
        <span>{post.issue.padStart(3, "0")}</span>
      </div>
      <div className="absolute inset-x-[8%] top-1/2 h-px bg-border sm:inset-x-[12%]" />
      <div className="absolute inset-x-[8%] top-1/2 flex -translate-y-1/2 items-center justify-between sm:inset-x-[12%]">
        {steps.map(([icon, label], index) => (
          <div key={label} className="flex min-w-0 flex-col items-center gap-3">
            <span
              className={`flex size-10 items-center justify-center rounded-sm border sm:size-14 ${index === 1 ? "border-accent bg-accent text-accent-foreground" : "border-border bg-surface text-muted"}`}
            >
              <AppIcon icon={icon} size={index === 1 ? 22 : 19} />
            </span>
            <span className="hidden max-w-24 truncate font-mono text-[0.62rem] tracking-[0.08em] text-muted uppercase sm:block">
              {label}
            </span>
          </div>
        ))}
      </div>
      <div className="absolute right-5 bottom-4 left-5 flex items-center justify-between font-mono text-[0.62rem] tracking-[0.1em] text-muted uppercase sm:inset-x-7 sm:bottom-6">
        <span>{post.category}</span>
        <span>{post.readTime}</span>
      </div>
    </div>
  )
}

function ArticleCta() {
  return (
    <section className="mx-auto mt-20 w-[calc(100%-2rem)] max-w-[900px] py-16 text-center md:mt-24 md:py-20">
      <p className="text-[0.7rem] font-medium tracking-[0.14em] text-muted uppercase">
        Put it into practice
      </p>
      <h2 className="mx-auto mt-5 max-w-[720px] text-[clamp(2.4rem,5vw,4.5rem)] leading-[0.94] font-medium tracking-[-0.06em]">
        Build the agent your team needs.
      </h2>
      <Link href="/auth/signup" className="mt-8 inline-flex">
        <Button variant="primary">Start for free</Button>
      </Link>
    </section>
  )
}

export default async function BlogArticlePage({
  params,
}: BlogArticlePageProps) {
  const { slug } = await params
  const post = getBlogPost(slug)
  if (!post) notFound()

  const content = getBlogArticleContent(post)
  const postIndex = blogPosts.findIndex((candidate) => candidate.slug === slug)
  const nextPost = blogPosts[(postIndex + 1) % blogPosts.length] ?? post

  return (
    <main className="marketing-link-scope min-h-screen bg-background text-foreground">
      <LandingHeader />
      <article>
        <header className="mx-auto w-[calc(100%-2rem)] max-w-[1300px] pt-20 md:pt-28">
          <Link
            href="/blog"
            className="inline-flex items-center gap-2 text-sm text-muted hover:text-foreground"
          >
            <AppIcon icon="arrow-left" size={15} />
            Back to blog
          </Link>

          <div className="mt-12 max-w-[1080px] md:mt-16">
            <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[0.7rem] tracking-[0.07em] uppercase">
              <span className="text-accent">{post.category}</span>
              <span className="text-muted" aria-hidden="true">
                ·
              </span>
              <time className="text-muted">{post.date}</time>
              <span className="text-muted" aria-hidden="true">
                ·
              </span>
              <span className="text-muted">{post.readTime}</span>
            </div>
            <h1 className="mt-6 text-[clamp(3rem,7vw,6.75rem)] leading-[0.89] font-medium tracking-[-0.07em]">
              {post.title}
            </h1>
            <p className="mt-7 max-w-[760px] text-lg leading-8 text-muted md:text-xl">
              {post.excerpt}
            </p>
            <p className="mt-7 text-sm text-muted">
              By <span className="text-foreground">{post.author}</span>
            </p>
          </div>

          <ArticleArtwork post={post} />
        </header>

        <div className="mx-auto mt-16 w-[calc(100%-2rem)] max-w-[760px] md:mt-24">
          <section
            aria-labelledby="article-summary-heading"
            className="rounded-sm border border-border bg-surface-secondary p-6 sm:p-8"
          >
            <h2
              id="article-summary-heading"
              className="text-[0.72rem] font-medium tracking-[0.14em] text-muted uppercase"
            >
              The short version
            </h2>
            <ul className="mt-5 space-y-3">
              {content.summary.map((item) => (
                <li key={item} className="flex gap-3 text-base leading-7">
                  <AppIcon
                    icon="check"
                    size={16}
                    className="mt-1.5 shrink-0 text-accent"
                  />
                  <span>{item}</span>
                </li>
              ))}
            </ul>
          </section>

          <div className="mt-16 space-y-16 md:mt-20 md:space-y-20">
            {content.sections.map((section) => (
              <section key={section.id} id={section.id}>
                <h2 className="text-[clamp(2rem,4vw,3.15rem)] leading-[1] font-medium tracking-[-0.05em]">
                  {section.heading}
                </h2>
                <div className="mt-6 space-y-6 text-[1.05rem] leading-8 text-muted">
                  {section.paragraphs.map((paragraph) => (
                    <p key={paragraph}>{paragraph}</p>
                  ))}
                </div>
                {section.points ? (
                  <ul className="mt-8 space-y-4 border-y border-border py-6">
                    {section.points.map((point) => (
                      <li
                        key={point}
                        className="flex gap-4 text-base leading-7"
                      >
                        <span
                          className="mt-2.5 size-1.5 shrink-0 rounded-full bg-accent"
                          aria-hidden="true"
                        />
                        <span>{point}</span>
                      </li>
                    ))}
                  </ul>
                ) : null}
              </section>
            ))}
          </div>

          <div className="mt-20 flex flex-col items-start justify-between gap-6 border-t border-border pt-8 sm:flex-row sm:items-center">
            <div>
              <p className="text-[0.7rem] tracking-[0.12em] text-muted uppercase">
                Continue reading
              </p>
              <p className="mt-2 max-w-[420px] text-lg font-medium">
                {nextPost.title}
              </p>
            </div>
            <Link
              href={`/blog/${nextPost.slug}`}
              className="inline-flex shrink-0 items-center gap-2 text-sm font-medium"
            >
              Next post
              <AppIcon icon="arrow-right" size={15} />
            </Link>
          </div>
        </div>
      </article>
      <ArticleCta />
      <LandingFooter />
    </main>
  )
}
