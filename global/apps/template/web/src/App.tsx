import { Link, Route, Switch } from "wouter"
import NotFound from "./pages/NotFound"
import Team from "./pages/Team"
import Welcome from "./pages/Welcome"

// App shell: nav + routes. wouter runs in browser-history mode (its
// default) — no hash URLs. Deep links like /team work because the Go server
// (hivycore/static.go) serves index.html for every path that isn't a real
// file, so the SPA boots and wouter matches the route.
//
// To add a screen: create a component in src/pages/ and add one <Route>
// here (plus a nav link if it should be reachable from the header). Keep
// the routeless <Route> last — it's the catch-all 404.
export default function App() {
  return (
    <main className="shell">
      <nav className="nav">
        <Link href="/" className={(active) => (active ? "active" : "")}>
          Welcome
        </Link>
        <Link href="/team" className={(active) => (active ? "active" : "")}>
          Team
        </Link>
      </nav>

      <Switch>
        <Route path="/" component={Welcome} />
        <Route path="/team" component={Team} />
        <Route component={NotFound} />
      </Switch>
    </main>
  )
}
