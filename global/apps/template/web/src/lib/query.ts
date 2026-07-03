import { QueryClient } from "@tanstack/react-query"
import { apiFetch } from "../api"

// The single QueryClient for the whole app, mounted once in main.tsx.
// Defaults tuned for small embedded Hivy apps:
//
// - retry: 1              — one retry smooths a transient blip without
//                           hammering the backend (or re-running a 401
//                           relaunch) on real failures.
// - refetchOnWindowFocus: false — the app lives in an iframe inside the Hivy
//                           frontend; focus flips constantly as users move
//                           around the host UI, and each flip would refetch.
// - staleTime: 30s        — sheet data changes on human timescales; serving
//                           a ≤30s-old cache on remount avoids redundant
//                           requests while staying fresh enough.
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 30_000,
    },
  },
})

// fetchJSON is the one HTTP helper query/mutation hooks should use. It is
// api.ts's apiFetch, so the 401 → top-window relaunch behavior runs before
// the error ever reaches React Query — a stale session both kicks off the
// re-launch flow and surfaces as a query error ("session expired").
export const fetchJSON = apiFetch
