import { Component, type ReactNode } from "react"

// Last-resort error boundary, mounted once at the root (main.tsx). If any
// component throws during render, this panel shows instead of a white screen
// — important because the app runs inside an iframe on the Hivy frontend,
// where a blank frame gives the user nothing to act on. Styles are inline on
// purpose: the boundary must render even if the stylesheet failed.
export class ErrorBoundary extends Component<
  { children: ReactNode },
  { error: Error | null }
> {
  state = { error: null as Error | null }

  static getDerivedStateFromError(error: Error) {
    return { error }
  }

  render() {
    if (this.state.error) {
      return (
        <div
          role="alert"
          style={{
            maxWidth: "640px",
            margin: "3rem auto",
            padding: "1.5rem",
            fontFamily: 'system-ui, -apple-system, "Segoe UI", sans-serif',
            lineHeight: 1.5,
          }}
        >
          <h1 style={{ fontSize: "1.2rem", margin: "0 0 0.5rem" }}>
            Something went wrong
          </h1>
          <p style={{ margin: "0 0 1rem", opacity: 0.75 }}>
            The app hit an unexpected error. Reloading usually fixes it.
          </p>
          <pre
            style={{
              padding: "0.75rem",
              borderRadius: "6px",
              background: "rgba(127, 127, 127, 0.12)",
              fontSize: "0.8rem",
              whiteSpace: "pre-wrap",
              overflowX: "auto",
            }}
          >
            {this.state.error.message}
          </pre>
          <button
            type="button"
            onClick={() => window.location.reload()}
            style={{
              padding: "0.4rem 0.9rem",
              borderRadius: "6px",
              border: "1px solid rgba(127, 127, 127, 0.4)",
              background: "transparent",
              color: "inherit",
              font: "inherit",
              cursor: "pointer",
            }}
          >
            Reload
          </button>
        </div>
      )
    }
    return this.props.children
  }
}
