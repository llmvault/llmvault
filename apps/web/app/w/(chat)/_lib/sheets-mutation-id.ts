/* ------------------------------------------------------------------ */
/* Self-echo suppression: every mutation carries a client-generated    */
/* mutation_id that the backend echoes on SSE events.                  */
/* ------------------------------------------------------------------ */

const recentMutationIds = new Set<string>()

export function newMutationId(): string {
  const id =
    typeof crypto !== "undefined" && "randomUUID" in crypto
      ? crypto.randomUUID()
      : `mut_${Date.now()}_${Math.random().toString(36).slice(2)}`
  recentMutationIds.add(id)
  if (recentMutationIds.size > 200) {
    for (const old of recentMutationIds) {
      if (recentMutationIds.size <= 100) break
      recentMutationIds.delete(old)
    }
  }
  return id
}

export function isOwnMutation(id: string | null | undefined): boolean {
  return Boolean(id && recentMutationIds.has(id))
}
