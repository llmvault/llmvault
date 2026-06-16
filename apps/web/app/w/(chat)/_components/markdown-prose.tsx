"use client"

import { Streamdown, type Components } from "streamdown"

const markdownComponents: Components = {
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
        lineNumbers={false}
      className={`hivy-markdown text-[14px] leading-6 ${
        muted ? "text-muted" : "text-foreground"
      }`}
    >
      {text}
    </Streamdown>
  )
}
