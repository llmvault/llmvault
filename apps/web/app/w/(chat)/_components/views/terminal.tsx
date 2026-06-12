"use client"

import { useRef, useState } from "react"
import { terminalLines } from "../../_lib/static-data"

type Line = { kind: "cmd" | "out" | "ok" | "err"; text: string }

const CANNED_OUTPUTS: Record<string, Line[]> = {
  ls: [
    { kind: "out", text: "apps  cmd  internal  scripts  go.mod  go.sum" },
  ],
  pwd: [{ kind: "out", text: "/Users/hivy/code/usehivy.com" }],
  "git status": [
    { kind: "out", text: "On branch main" },
    { kind: "ok", text: "nothing to commit, working tree clean" },
  ],
  "go test ./internal/handler": [
    {
      kind: "out",
      text: "ok      github.com/usehivy/hivy/internal/handler   2.418s",
    },
  ],
}

export function TerminalView() {
  const [history, setHistory] = useState<Line[]>(
    // Drop the trailing empty prompt from the static sample; the live
    // input row below acts as the prompt.
    terminalLines.filter((line, index) => index !== terminalLines.length - 1)
  )
  const [value, setValue] = useState("")
  const inputRef = useRef<HTMLInputElement | null>(null)

  const run = () => {
    const command = value.trim()
    if (!command) return
    const output =
      CANNED_OUTPUTS[command] ??
      ([
        {
          kind: "err",
          text: `zsh: command not found: ${command.split(" ")[0]} (static exploration)`,
        },
      ] satisfies Line[])
    setHistory((current) => [...current, { kind: "cmd", text: command }, ...output])
    setValue("")
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b border-border px-4 py-2 text-sm text-muted">
        <span className="font-medium text-foreground">zsh</span>
        <span>~/code/usehivy.com</span>
      </div>
      <div
        className="min-h-0 flex-1 cursor-text overflow-y-auto bg-surface px-4 py-3 font-mono text-[13px] leading-6"
        onClick={() => inputRef.current?.focus()}
      >
        {history.map((line, index) =>
          line.kind === "cmd" ? (
            <div key={index} className="flex gap-2">
              <span className="text-accent">❯</span>
              <span>{line.text}</span>
            </div>
          ) : (
            <div
              key={index}
              className={
                line.kind === "ok"
                  ? "text-success"
                  : line.kind === "err"
                    ? "text-danger"
                    : "text-muted"
              }
            >
              {line.text}
            </div>
          )
        )}

        <div className="flex gap-2">
          <span className="text-accent">❯</span>
          <input
            ref={inputRef}
            type="text"
            value={value}
            spellCheck={false}
            autoCapitalize="off"
            autoComplete="off"
            aria-label="Terminal input"
            onChange={(event) => setValue(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") run()
            }}
            className="min-w-0 flex-1 bg-transparent font-mono outline-none"
          />
        </div>
      </div>
    </div>
  )
}
