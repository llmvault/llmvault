import type { RefObject } from "react"
import type { PanelImperativeHandle } from "react-resizable-panels"

export function resizePanelIfMounted(
  panelRef: RefObject<PanelImperativeHandle | null>,
  expectedHandle: PanelImperativeHandle,
  size: number | string
) {
  if (panelRef.current !== expectedHandle) return false
  expectedHandle.resize(size)
  return true
}
