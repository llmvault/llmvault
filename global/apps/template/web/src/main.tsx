import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { QueryClientProvider } from "@tanstack/react-query"
import App from "./App"
import { ErrorBoundary } from "./components/ErrorBoundary"
import { queryClient } from "./lib/query"
import "./styles.css"

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
