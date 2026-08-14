import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import {
  configureDesktopRuntime,
  deliverDesktopMessage,
  isDesktopApp,
  streamDesktopSession,
} from "./bridge"

beforeEach(() => {
  ;(globalThis as unknown as Record<string, unknown>).window = globalThis
})

afterEach(() => {
  delete (window as unknown as Record<string, unknown>).__TAURI__
})

describe("desktop bridge", () => {
  it("is disabled in a normal browser", () => {
    expect(isDesktopApp()).toBe(false)
  })

  it("configures and delivers through the Tauri runtime command", async () => {
    const invoke = vi
      .fn()
      .mockResolvedValueOnce({
        desktop: true,
        runtime_base_url: "http://127.0.0.1:37080",
        runtime_ready: true,
      })
      .mockResolvedValueOnce({ status: 200, body: {} })
      .mockResolvedValueOnce({
        status: 200,
        body: { turn_id: "turn-1", stream_id: "stream-1" },
      })
    ;(window as unknown as Record<string, unknown>).__TAURI__ = {
      core: { invoke },
    }

    await configureDesktopRuntime("agent-1", {
      definition: { agent: { name: "Hivy" } },
    })
    const delivery = await deliverDesktopMessage<{
      turn_id: string
      stream_id: string
    }>("agent-1", "session-1", { text: "hello" })

    expect(delivery.turn_id).toBe("turn-1")
    expect(invoke).toHaveBeenNthCalledWith(2, "runtime_request", {
      request: {
        method: "PUT",
        path: "/desktop/agents/agent-1/config",
        body: { definition: { agent: { name: "Hivy" } } },
      },
    })
    expect(invoke).toHaveBeenNthCalledWith(3, "runtime_request", {
      request: {
        method: "POST",
        path: "/desktop/agents/agent-1/sessions/session-1/messages",
        body: { text: "hello" },
      },
    })
  })

  it("streams native runtime frames through a Tauri channel", async () => {
    class TestChannel<T> {
      onmessage: (message: T) => void = () => undefined
    }
    const frame = {
      sessionId: "session-1",
      event: "token",
      id: "4",
      data: { text: "hello", sequence: 4 },
    }
    const invoke = vi.fn(
      async (_command: string, args?: Record<string, unknown>) => {
        const channel = args?.onEvent as TestChannel<typeof frame>
        channel.onmessage(frame)
      }
    )
    ;(window as unknown as Record<string, unknown>).__TAURI__ = {
      core: { invoke, Channel: TestChannel },
    }
    const received: typeof frame[] = []

    await streamDesktopSession("session-1", "turn-1", (event) => {
      received.push(event as typeof frame)
    })

    expect(received).toEqual([frame])
    expect(invoke).toHaveBeenCalledWith("runtime_session_stream", {
      sessionId: "session-1",
      turnId: "turn-1",
      onEvent: expect.any(TestChannel),
    })
  })
})
