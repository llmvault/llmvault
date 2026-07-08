// Pure helpers for team provisioning (org plugins + knowledge sources granted
// to a team). Kept dependency-free so they're covered by plain vitest specs
// without a DOM/React renderer.

// enabledIdSet collects the truthy `id` fields from a list of team-scoped
// resource rows (enabled plugins / granted rag sources) into a lookup set.
export function enabledIdSet(items: { id?: string }[] | undefined): Set<string> {
  const set = new Set<string>()
  for (const item of items ?? []) {
    if (item.id) set.add(item.id)
  }
  return set
}

// isProvisioned reports whether an org-level resource id is present in the
// team's enabled/granted set. Guards against an undefined id (unsaved rows).
export function isProvisioned(
  id: string | undefined,
  enabledIds: Set<string>
): boolean {
  if (!id) return false
  return enabledIds.has(id)
}

// nextEnabledSet returns a new Set with `id` added (on) or removed (off),
// leaving `current` untouched. Used to drive the optimistic toggle before the
// mutation settles.
export function nextEnabledSet(
  current: Set<string>,
  id: string,
  on: boolean
): Set<string> {
  const next = new Set(current)
  if (on) next.add(id)
  else next.delete(id)
  return next
}
