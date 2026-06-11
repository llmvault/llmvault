import { useCallback, useEffect, useRef, useState } from "react"
import { fetchEventSource } from "@microsoft/fetch-event-source"
import {
  DEFAULT_RECONNECT_CONFIG,
  StreamBuffer,
  decideReconnect,
  proxiedStreamURL,
} from "@/lib/sessions/stream"
import {
  appendLiveThinkingEvent,
  appendLiveTokenEvent,
  completeTrailingLiveAssistant,
  eventKind,
  isHiddenSessionEvent,
  parseStreamFrame,
  reconcileLiveFinalEvent,
  streamFrameToSessionEvent,
} from "@/lib/sessions/normalize"
import type { StreamState } from "@/lib/sessions/types"

/**
 * Owns the live SSE state for web session turns: the per-session `streams` map,
 * the abort controllers that drive reconnect/abort, and `startSessionStream`,
 * which runs the reconnect loop and accumulates tokens/thinking/tool events.
 *
 * `refetchPersistedSession` is invoked on stream completion so the durable
 * events are refetched before the live state is pruned.
 */
export function useSessionStream(refetchPersistedSession: (
  sessionID: string
) => void) {
  const [streams, setStreams] = useState<Record<string, StreamState>>({})
  const streamControllersRef = useRef<Record<string, AbortController>>({})

  const pruneStream = useCallback((sessionID: string) => {
    setStreams((current) => {
      if (!(sessionID in current)) return current
      const next = { ...current }
      delete next[sessionID]
      return next
    })
  }, [])

  const startSessionStream = useCallback(
    async (sessionID: string, sessionStreamURL?: string) => {
      if (!sessionStreamURL) return

      streamControllersRef.current[sessionID]?.abort()
      const controller = new AbortController()
      streamControllersRef.current[sessionID] = controller
      setStreams((current) => ({
        ...current,
        [sessionID]: { text: "", events: [], isStreaming: true },
      }))

      const buffer = new StreamBuffer()
      let sequence = 0
      let reachedTerminal = false
      let attempt = 0
      let isReplay = false
      const startedAt = Date.now()

      // Resolve the backend-provided stream path into a URL on our own
      // cookie-auth proxy. Absolute URLs are rejected so the raw bearer access
      // token is never attached to a cross-origin SSE request from client JS;
      // the proxy authenticates the browser via the __session cookie and the
      // HMAC-signed ?token= already baked into the path. Any rejection
      // is caught here so isStreaming is cleared instead of sticking.
      let streamURL: string
      try {
        streamURL = proxiedStreamURL(sessionStreamURL)
      } catch (err) {
        setStreams((current) => ({
          ...current,
          [sessionID]: {
            ...(current[sessionID] ?? { events: [] }),
            text: "",
            events: [],
            isStreaming: false,
            error:
              err instanceof Error
                ? err.message
                : "Failed to resolve stream URL",
          },
        }))
        if (streamControllersRef.current[sessionID] === controller) {
          delete streamControllersRef.current[sessionID]
        }
        return
      }

      const commitToken = (tokenText: string) => {
        if (!tokenText) return
        const wasReplay = isReplay
        const newText = wasReplay
          ? buffer.replayToken(tokenText)
          : buffer.liveToken(tokenText)
        // While catching up on a replay, suppress re-rendering already-seen
        // tokens; only append the genuinely-new suffix to the live event list.
        if (wasReplay && newText === "") return
        // Once a replayed token yields new text we have caught up to the live
        // tail; subsequent tokens on this connection are appended verbatim.
        if (wasReplay) isReplay = false
        sequence += 1
        const appended = wasReplay ? newText : tokenText
        setStreams((current) => {
          const existing = current[sessionID] ?? {
            text: "",
            events: [],
            isStreaming: true,
          }
          return {
            ...current,
            [sessionID]: {
              ...existing,
              text: buffer.text,
              events: appendLiveTokenEvent(
                existing.events,
                sessionID,
                appended,
                sequence
              ),
              isStreaming: true,
            },
          }
        })
      }

      // Run a single SSE connection. Returns true if it ended on a terminal
      // event (done/final-complete) or an unrecoverable error; false if the
      // connection ended/closed while the turn was plausibly still running.
      const runConnection = async (): Promise<void> => {
        // The stream is always served through our cookie-auth proxy: the
        // browser sends its __session cookie (credentials: include) and the
        // proxy injects the bearer server-side. No access token ever touches
        // client JS.
        await fetchEventSource(streamURL, {
          method: "GET",
          headers: { Accept: "text/event-stream" },
          credentials: "include",
          signal: controller.signal,
          openWhenHidden: true,
          async onopen(response) {
            const contentType = response.headers.get("content-type") ?? ""
            if (response.ok && contentType.includes("text/event-stream")) return
            let message = `Stream failed with HTTP ${response.status}`
            try {
              const envelope = (await response.json()) as { error?: string }
              if (envelope.error) message = envelope.error
            } catch {
              /* non-JSON stream error */
            }
            throw new Error(message)
          },
          onmessage(event) {
            if (!event.data) return
            const frame = parseStreamFrame(event.data)
            if (!frame) return

            if (event.event === "token" && typeof frame.text === "string") {
              commitToken(frame.text)
              return
            }
            if (event.event === "final" && typeof frame.text === "string") {
              const finalText = frame.text
              sequence += 1
              buffer.setFinal(finalText)
              isReplay = false
              setStreams((current) => {
                const existing = current[sessionID] ?? {
                  text: "",
                  events: [],
                  isStreaming: true,
                }
                return {
                  ...current,
                  [sessionID]: {
                    ...existing,
                    text: buffer.text,
                    events: reconcileLiveFinalEvent(
                      existing.events,
                      sessionID,
                      finalText,
                      sequence
                    ),
                    isStreaming: true,
                  },
                }
              })
              return
            }
            if (event.event === "error") {
              reachedTerminal = true
              const message =
                typeof frame.message === "string"
                  ? frame.message
                  : "The response stream failed."
              setStreams((current) => ({
                ...current,
                [sessionID]: {
                  ...(current[sessionID] ?? { events: [] }),
                  text: buffer.text,
                  events: completeTrailingLiveAssistant(
                    current[sessionID]?.events ?? []
                  ),
                  isStreaming: false,
                  error: message,
                },
              }))
              controller.abort()
              return
            }
            if (event.event === "done") {
              reachedTerminal = true
              controller.abort()
              return
            }
            sequence += 1
            const liveEvent = streamFrameToSessionEvent(
              sessionID,
              event.event,
              frame,
              sequence
            )
            const isThinking = eventKind(liveEvent) === "thinking"
            setStreams((current) => {
              const existing = current[sessionID] ?? {
                text: buffer.text,
                events: [],
                isStreaming: true,
              }
              const completedEvents = completeTrailingLiveAssistant(
                existing.events
              )
              // Coalesce per-frame thinking deltas into the trailing thinking
              // event so the live events array stays bounded.
              const nextEvents = isHiddenSessionEvent(liveEvent)
                ? completedEvents
                : isThinking
                  ? appendLiveThinkingEvent(completedEvents, liveEvent)
                  : [...completedEvents, liveEvent]
              return {
                ...current,
                [sessionID]: {
                  ...existing,
                  text: existing.text || buffer.text,
                  events: nextEvents,
                  isStreaming: true,
                },
              }
            })
          },
          // Returning nothing/undefined here keeps fetch-event-source from
          // auto-retrying; we drive reconnection ourselves below so the backoff
          // and "is the turn still running" decision live in one tested place.
          onerror(error) {
            throw error
          },
        })
      }

      try {
        // Reconnect loop: a clean server close (e.g. an upstream proxy idle/
        // total timeout) ends the body without a terminal event, so we resume
        // against the same signed stream URL while the turn is plausibly still
        // running. The broker replays buffered history, which StreamBuffer
        // dedupes against what we already rendered.
        for (;;) {
          try {
            await runConnection()
          } catch (error) {
            if (controller.signal.aborted) break
            const decision = decideReconnect(
              {
                reachedTerminal,
                attempt,
                elapsedMs: Date.now() - startedAt,
              },
              DEFAULT_RECONNECT_CONFIG
            )
            if (!decision.reconnect) {
              const message =
                error instanceof Error
                  ? error.message
                  : "The response stream failed."
              setStreams((current) => ({
                ...current,
                [sessionID]: {
                  ...(current[sessionID] ?? { events: [] }),
                  text: buffer.text,
                  events: completeTrailingLiveAssistant(
                    current[sessionID]?.events ?? []
                  ),
                  isStreaming: false,
                  error: message,
                },
              }))
              break
            }
            attempt += 1
            isReplay = true
            buffer.beginReplay()
            await delay(decision.delayMs, controller.signal)
            if (controller.signal.aborted) break
            continue
          }

          // The connection ended cleanly. If we saw a terminal event, the turn
          // is finished. Otherwise the body was cut mid-turn — resume.
          if (controller.signal.aborted || reachedTerminal) break
          const decision = decideReconnect(
            { reachedTerminal, attempt, elapsedMs: Date.now() - startedAt },
            DEFAULT_RECONNECT_CONFIG
          )
          if (!decision.reconnect) break
          attempt += 1
          isReplay = true
          buffer.beginReplay()
          await delay(decision.delayMs, controller.signal)
          if (controller.signal.aborted) break
        }
      } finally {
        if (streamControllersRef.current[sessionID] === controller) {
          delete streamControllersRef.current[sessionID]
        }
        setStreams((current) => {
          const existing = current[sessionID]
          // Do NOT resurrect a stream entry that the persisted-events cleanup
          // effect already pruned. Only mark the still-present entry as
          // no longer streaming.
          if (!existing) return current
          return {
            ...current,
            [sessionID]: {
              ...existing,
              text: existing.text || buffer.text,
              events: completeTrailingLiveAssistant(existing.events),
              isStreaming: false,
            },
          }
        })
        // On stream completion, refetch the persisted events so the turn's
        // answer is loaded from the backend before the live state is cleared.
        // This is what keeps a previous turn's answer visible when a follow-up
        // message resets the live stream state.
        refetchPersistedSession(sessionID)
      }
    },
    [refetchPersistedSession]
  )

  useEffect(() => {
    const controllers = streamControllersRef.current
    return () => {
      for (const controller of Object.values(controllers)) {
        controller.abort()
      }
      streamControllersRef.current = {}
    }
  }, [])

  return { streams, startSessionStream, pruneStream }
}

function delay(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal?.aborted) {
      resolve()
      return
    }
    const timer = setTimeout(() => {
      signal?.removeEventListener("abort", onAbort)
      resolve()
    }, ms)
    const onAbort = () => {
      clearTimeout(timer)
      resolve()
    }
    signal?.addEventListener("abort", onAbort, { once: true })
  })
}
