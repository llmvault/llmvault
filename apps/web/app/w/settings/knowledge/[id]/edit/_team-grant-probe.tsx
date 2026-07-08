"use client"

import { useEffect, useRef } from "react"
import { $api } from "@/lib/api/hooks"

// TeamGrantProbe checks whether `sourceId` is currently granted to `teamId`
// and reports the result exactly once via onResult. Renders nothing.
//
// There's no endpoint that lists which teams a source is granted to, so the
// edit page mounts one of these per team to probe each team's own
// rag-sources list. Hooks can't be called in a loop, hence one component
// instance per team rather than one query with a dynamic team list.
export function TeamGrantProbe({
  teamId,
  sourceId,
  onResult,
}: {
  teamId: string
  sourceId: string
  onResult: (teamId: string, granted: boolean) => void
}) {
  const query = $api.useQuery(
    "get",
    "/v1/orgs/current/teams/{teamID}/rag-sources",
    { params: { path: { teamID: teamId } } }
  )
  const reported = useRef(false)

  useEffect(() => {
    if (reported.current || !query.data) return
    reported.current = true
    const granted = (query.data.data ?? []).some((s) => s.id === sourceId)
    onResult(teamId, granted)
  }, [query.data, teamId, sourceId, onResult])

  return null
}
