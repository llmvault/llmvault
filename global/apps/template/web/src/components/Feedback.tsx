// Shared loading / error primitives. Every screen that fetches data renders
// these for its isPending and isError states — copy this pattern, don't
// leave queries without visible loading/error handling.

export function Loading({ label = "Loading…" }: { label?: string }) {
  return <p className="muted">{label}</p>
}

export function ErrorNotice({ error }: { error: unknown }) {
  const message = error instanceof Error ? error.message : String(error)
  return <p className="error">Something went wrong: {message}</p>
}
