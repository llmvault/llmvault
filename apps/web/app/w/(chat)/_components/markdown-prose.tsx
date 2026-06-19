"use client"

import { Link } from "@heroui/react"
import { Streamdown, type Components } from "streamdown"
import { cn } from "@/lib/utils"

const markdownComponents: Components = {
  a({ children, className, href, node: _node }) {
    if (!href || href === "streamdown:incomplete-link") {
      return <span className={cn("wrap-anywhere", className)}>{children}</span>
    }

    const external = href.startsWith("http")

    return (
      <Link
        href={href}
        target={external ? "_blank" : undefined}
        rel={external ? "noreferrer" : undefined}
        className={cn(
          "font-medium wrap-anywhere underline-offset-2 hover:underline",
          className
        )}
      >
        {children}
      </Link>
    )
  },
  table({ children, className, node: _node, ...props }) {
    return (
      <div className="hivy-markdown-table-wrap">
        <table className={className} {...props}>
          {children}
        </table>
      </div>
    )
  },
  pre({ children, className, node: _node, ...props }) {
    return (
      <pre className={`hivy-markdown-code-block ${className ?? ""}`} {...props}>
        {children}
      </pre>
    )
  },
  code({ children, className, node: _node, ...props }) {
    return (
      <code className={className} {...props}>
        {children}
      </code>
    )
  },
}

export function MarkdownProse({
  text,
  streaming = false,
  muted = false,
}: {
  text: string
  streaming?: boolean
  muted?: boolean
}) {
  return (
    <Streamdown
      mode={streaming ? "streaming" : "static"}
      isAnimating={streaming}
      components={markdownComponents}
      controls={false}
      linkSafety={{ enabled: false }}
      lineNumbers={false}
      className={`hivy-markdown text-[14px] leading-6 ${
        muted ? "text-muted" : "text-foreground"
      }`}
    >
      {text}
    </Streamdown>
  )
}
