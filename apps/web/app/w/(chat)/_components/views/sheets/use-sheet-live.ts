"use client"

import { useEffect } from "react"
import { fetchEventSource } from "@microsoft/fetch-event-source"
import { useQueryClient, type QueryClient } from "@tanstack/react-query"
import { toast } from "@heroui/react"
import { apiUrl } from "@/lib/api/client"
import {
  applyRowsChangedEvent,
  isOwnMutation,
  mintLiveToken,
  sheetKeys,
  type SheetImportJob,
  type SheetLiveEvent,
  type SheetStructure,
} from "@/app/w/(chat)/_lib/sheets"

/** Server ended the stream (token expiry / resync); reconnect immediately. */
class StreamClosedError extends Error {
  constructor(readonly immediate: boolean) {
    super("sheet live stream closed")
    this.name = "StreamClosedError"
  }
}

function parseEventData(raw: string): Record<string, unknown> | null {
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw) as unknown
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>
    }
    return null
  } catch {
    return null
  }
}

/** Invalidate structure + every cached rows window for the sheet's pages. */
function refetchSheet(queryClient: QueryClient, sheetId: string) {
  void queryClient.invalidateQueries({
    queryKey: sheetKeys.structure(sheetId),
  })
  const structure = queryClient.getQueryData<SheetStructure>(
    sheetKeys.structure(sheetId)
  )
  for (const page of structure?.pages ?? []) {
    if (!page.id) continue
    void queryClient.invalidateQueries({
      queryKey: sheetKeys.rowsPrefix(page.id),
    })
  }
}

function handleSheetEvent(
  queryClient: QueryClient,
  sheetId: string,
  event: SheetLiveEvent
) {
  switch (event.type) {
    case "rows_changed":
      if (isOwnMutation(event.mutation_id)) return
      applyRowsChangedEvent(queryClient, event)
      break
    case "fields_changed":
    case "pages_changed":
      if (isOwnMutation(event.mutation_id)) return
      void queryClient.invalidateQueries({
        queryKey: sheetKeys.structure(sheetId),
      })
      if (event.type === "fields_changed" && event.page_id) {
        void queryClient.invalidateQueries({
          queryKey: sheetKeys.rowsPrefix(event.page_id),
        })
      }
      break
    case "import_progress": {
      const jobId = event.job_id
      if (jobId) {
        queryClient.setQueryData<SheetImportJob>(
          sheetKeys.importJob(jobId),
          (prev) => ({
            ...(prev ?? { id: jobId }),
            status: event.status ?? prev?.status,
            processed_rows: event.processed_rows ?? prev?.processed_rows,
            total_rows: event.total_rows ?? prev?.total_rows,
          })
        )
      }
      // No rows_changed events arrive during an import — refetch on completion.
      if (event.status === "completed") {
        if (event.page_id) {
          void queryClient.invalidateQueries({
            queryKey: sheetKeys.rowsPrefix(event.page_id),
          })
          void queryClient.invalidateQueries({
            queryKey: sheetKeys.structure(sheetId),
          })
        } else {
          refetchSheet(queryClient, sheetId)
        }
      }
      break
    }
    case "operation_reverted":
      if (event.page_id) {
        void queryClient.invalidateQueries({
          queryKey: sheetKeys.rowsPrefix(event.page_id),
        })
        void queryClient.invalidateQueries({
          queryKey: sheetKeys.operations(event.page_id),
        })
      }
      void queryClient.invalidateQueries({
        queryKey: sheetKeys.structure(sheetId),
      })
      if (!isOwnMutation(event.mutation_id)) {
        toast.success("A sheet operation was reverted")
      }
      break
  }
}

/**
 * Live sheet updates over SSE, connected directly to the Go API
 * (NEXT_PUBLIC_HIVY_API_URL) with a short-lived live token minted through
 * the proxy. Reconnects with a fresh token on expiry, close, or error, and
 * heals missed events by refetching the sheet's cached windows on reconnect.
 */
export function useSheetLive(sheetId: string | null | undefined) {
  const queryClient = useQueryClient()

  useEffect(() => {
    if (!sheetId) return

    const controller = new AbortController()
    let connectedBefore = false

    const connect = async () => {
      const minted = await mintLiveToken(sheetId)
      const token = minted.token
      if (!token) throw new Error("Live token response was empty")

      await fetchEventSource(
        apiUrl(`/v1/sheets/${encodeURIComponent(sheetId)}/live`),
        {
          method: "GET",
          headers: {
            Accept: "text/event-stream",
            Authorization: `Bearer ${token}`,
          },
          credentials: "omit",
          openWhenHidden: true,
          signal: controller.signal,
          async onopen(response) {
            const contentType = response.headers.get("content-type") ?? ""
            if (!response.ok || !contentType.includes("text/event-stream")) {
              throw new Error(
                `Sheet live stream failed with HTTP ${response.status}`
              )
            }
            if (connectedBefore) refetchSheet(queryClient, sheetId)
            connectedBefore = true
          },
          onmessage(message) {
            if (message.event === "sheet.control") {
              const data = parseEventData(message.data)
              const kind = typeof data?.type === "string" ? data.type : ""
              if (kind === "token_expired") throw new StreamClosedError(true)
              if (kind === "resync") refetchSheet(queryClient, sheetId)
              return
            }
            if (message.event === "sheet.event") {
              const data = parseEventData(message.data)
              if (data) {
                handleSheetEvent(
                  queryClient,
                  sheetId,
                  data as unknown as SheetLiveEvent
                )
              }
            }
          },
          onclose() {
            // Server closes after token expiry / resync; never let
            // fetchEventSource retry with the stale token.
            throw new StreamClosedError(true)
          },
          onerror(error) {
            // Bail out of fetchEventSource's internal retry; the outer loop
            // re-mints the token and reconnects with backoff.
            throw error
          },
        }
      )
    }

    const run = async () => {
      while (!controller.signal.aborted) {
        const startedAt = Date.now()
        let immediate = false
        try {
          await connect()
        } catch (error) {
          if (controller.signal.aborted) return
          immediate = error instanceof StreamClosedError && error.immediate
        }
        if (controller.signal.aborted) return
        // Token expiry closes ~10 minutes in — reconnect immediately. If the
        // stream died right after opening, back off to avoid a hot loop.
        if (!immediate || Date.now() - startedAt < 5000) {
          await new Promise((resolve) =>
            setTimeout(resolve, 2000 + Math.random() * 2000)
          )
        }
      }
    }

    void run()
    return () => controller.abort()
  }, [queryClient, sheetId])
}
