import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { QueryClientProvider } from "@tanstack/react-query"
import App from "./App"
import { ErrorBoundary } from "./components/ErrorBoundary"
import { queryClient } from "./lib/query"
import { startRealtime } from "./lib/realtime"
import "./styles.css"

// Realtime infrastructure — DO NOT MODIFY OR REMOVE. This opens the single
// live stream that keeps every screen's data fresh automatically (see
// src/lib/realtime.ts). Because of it, apps must never poll or add refresh
// buttons; reading rows through useRows is enough.
startRealtime(queryClient)

// Root order matters: the provider wraps the boundary so a crashed tree can
// remount into a working query cache; the boundary wraps App so any render
// error shows a friendly panel instead of white-screening the iframe.
createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ErrorBoundary>
        <App />
      </ErrorBoundary>
    </QueryClientProvider>
  </StrictMode>
)
