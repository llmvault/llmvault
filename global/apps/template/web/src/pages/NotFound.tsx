import { Link } from "wouter"

// Catch-all 404 — rendered by the routeless <Route> at the bottom of App's
// <Switch>. The server always serves index.html for unknown paths (SPA
// fallback), so unknown URLs land here instead of a server error page.
export default function NotFound() {
  return (
    <section>
      <h1>Page not found</h1>
      <p className="muted">There's nothing at this address.</p>
      <p>
        <Link href="/">Back to the start</Link>
      </p>
    </section>
  )
}
