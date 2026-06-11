"use client"

import { useCallback } from "react"
import { useObserveScrollPosition } from "react-scroll-to-bottom"

export function EventsScrollObserver({
  onNearTop,
}: {
  onNearTop: () => void
}) {
  useObserveScrollPosition(
    useCallback(
      ({ scrollTop }: { scrollTop: number }) => {
        if (scrollTop < 96) onNearTop()
      },
      [onNearTop]
    )
  )
  return null
}
