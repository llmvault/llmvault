"use client"

type Invoke = <T>(command: string, args?: Record<string, unknown>) => Promise<T>

interface Channel<T> {
  onmessage: (message: T) => void
}

interface ChannelConstructor {
  new <T>(): Channel<T>
}

interface TauriCore {
  invoke?: Invoke
  Channel?: ChannelConstructor
}

export interface DesktopInfo {
  desktop: true
  runtime_base_url: string
  runtime_ready: boolean
}

export interface DesktopRuntimeResponse<T = unknown> {
  status: number
  body: T
}

export interface DesktopSessionStreamFrame {
  sessionId: string
  event: string
  id: string
  data: unknown
}

function tauriCore(): TauriCore | null {
  if (typeof window === "undefined") return null
  return (
    window as unknown as {
      __TAURI__?: { core?: TauriCore }
    }
  ).__TAURI__?.core ?? null
}

function tauriInvoke(): Invoke | null {
  return tauriCore()?.invoke ?? null
}

export function isDesktopApp(): boolean {
  return tauriInvoke() !== null
}

export async function desktopInfo(): Promise<DesktopInfo> {
  const invoke = tauriInvoke()
  if (!invoke) throw new Error("Hivy desktop bridge is unavailable")
  return invoke<DesktopInfo>("desktop_info")
}

export async function desktopRuntimeRequest<T = unknown>(
  method: string,
  path: string,
  body?: unknown
): Promise<DesktopRuntimeResponse<T>> {
  const invoke = tauriInvoke()
  if (!invoke) throw new Error("Hivy desktop bridge is unavailable")
  const response = await invoke<DesktopRuntimeResponse<T>>("runtime_request", {
    request: { method, path, body },
  })
  if (response.status < 200 || response.status >= 300) {
    const detail =
      typeof response.body === "string"
        ? response.body
        : JSON.stringify(response.body)
    throw new Error(`Local runtime rejected the request (${response.status}): ${detail}`)
  }
  return response
}

export async function configureDesktopRuntime(
  agentId: string,
  config: unknown
): Promise<void> {
  const info = await desktopInfo()
  if (!info.runtime_ready) {
    throw new Error("The local Hivy runtime is still starting. Try again in a moment.")
  }
  await desktopRuntimeRequest(
    "PUT",
    `/desktop/agents/${encodeURIComponent(agentId)}/config`,
    config
  )
}

export async function deliverDesktopMessage<T>(
  agentId: string,
  sessionId: string,
  runtimeRequest: unknown
): Promise<T> {
  const response = await desktopRuntimeRequest<T>(
    "POST",
    `/desktop/agents/${encodeURIComponent(agentId)}/sessions/${encodeURIComponent(sessionId)}/messages`,
    runtimeRequest
  )
  return response.body
}

export async function streamDesktopSession(
  sessionId: string,
  turnId: string,
  onEvent: (frame: DesktopSessionStreamFrame) => void
): Promise<void> {
  const core = tauriCore()
  if (!core?.invoke || !core.Channel) {
    throw new Error("Hivy desktop streaming bridge is unavailable")
  }
  const channel = new core.Channel<DesktopSessionStreamFrame>()
  channel.onmessage = onEvent
  await core.invoke("runtime_session_stream", {
    sessionId,
    turnId,
    onEvent: channel,
  })
}
