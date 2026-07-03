// Data hooks — the only way components talk to the backend. One hook per
// endpoint, built on useQuery/useMutation + fetchJSON. Components never call
// fetch or useEffect for data; they render hook state (isPending / isError /
// data) instead.
import { useQuery } from "@tanstack/react-query"
import type { Me, SheetStructure } from "../api"
import { fetchJSON } from "../lib/query"

/** The signed-in user (GET /api/me). */
export function useMe() {
  return useQuery({
    queryKey: ["me"],
    queryFn: () => fetchJSON<Me>("/api/me"),
  })
}

/** The bound sheet's structure: pages, fields, row counts (GET /api/pages). */
export function usePages() {
  return useQuery({
    queryKey: ["pages"],
    queryFn: () => fetchJSON<SheetStructure>("/api/pages"),
  })
}

// Mutation pattern — copy this shape when your API grows write endpoints.
// The template backend has no write route yet, so this stays a comment
// rather than pointing at an endpoint that doesn't exist:
//
//   import { useMutation, useQueryClient } from "@tanstack/react-query"
//
//   export function useCreateTodo() {
//     const queryClient = useQueryClient()
//     return useMutation({
//       mutationFn: (input: { title: string }) =>
//         fetchJSON<Todo>("/api/todos", { method: "POST", body: input }),
//       // Invalidate every query whose data the write could change, so the
//       // UI refetches instead of showing stale rows.
//       onSuccess: () =>
//         queryClient.invalidateQueries({ queryKey: ["todos"] }),
//     })
//   }
//
// In a component: const create = useCreateTodo()
//   create.mutate({ title }) — render create.isPending / create.isError.
