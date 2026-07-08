// teamGrantDiff computes which team ids need to be granted vs revoked to move
// from the currently-granted set to the selected set. Order follows each
// input array's own order (current for revoke, selected for grant); each id
// is deduped so repeats in the input don't produce repeated work.
export function teamGrantDiff(
  current: string[],
  selected: string[]
): { grant: string[]; revoke: string[] } {
  const currentSet = new Set(current)
  const selectedSet = new Set(selected)

  const grant: string[] = []
  const seenGrant = new Set<string>()
  for (const id of selected) {
    if (!currentSet.has(id) && !seenGrant.has(id)) {
      seenGrant.add(id)
      grant.push(id)
    }
  }

  const revoke: string[] = []
  const seenRevoke = new Set<string>()
  for (const id of current) {
    if (!selectedSet.has(id) && !seenRevoke.has(id)) {
      seenRevoke.add(id)
      revoke.push(id)
    }
  }

  return { grant, revoke }
}
