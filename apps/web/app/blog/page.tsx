import type { Metadata } from "next"
import { BlogPage as BlogIndexPage } from "./_components/blog-page"

export const metadata: Metadata = {
  title: "Hivy blog",
  description:
    "Practical notes about AI agents, models, company knowledge, workflows, permissions, and the systems behind reliable agent work.",
}

export default function BlogPage() {
  return <BlogIndexPage />
}
