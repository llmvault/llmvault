import { Spinner } from "@heroui/react"
import { useCallback, useEffect, useLayoutEffect, useRef } from "react"

const HISTORY_TOP_LOAD_THRESHOLD = 160
const HISTORY_SCROLL_ARM_DELTA = 2

export function shouldArmHistoryPagination(
  previousScrollTop: number,
  nextScrollTop: number
) {
  return nextScrollTop < previousScrollTop - HISTORY_SCROLL_ARM_DELTA
}

export function SessionHistoryTopLoader({
  hasMore,
  isFetching,
  loadedEventCount,
  onLoadMore,
}: {
  hasMore: boolean
  isFetching: boolean
  loadedEventCount: number
  onLoadMore: () => Promise<unknown>
}) {
  const markerRef = useRef<HTMLDivElement | null>(null)
  const loadingRef = useRef(false)
  const armedRef = useRef(false)
  const lastScrollTopRef = useRef<number | null>(null)
  const restoreRef = useRef<{
    scrollPanel: HTMLElement
    scrollHeight: number
    scrollTop: number
    loadedEventCount: number
  } | null>(null)

  const loadMore = useCallback(() => {
    if (!armedRef.current || !hasMore || isFetching || loadingRef.current) {
      return
    }

    const scrollPanel = markerRef.current?.parentElement
    if (!scrollPanel) return

    restoreRef.current = {
      scrollPanel,
      scrollHeight: scrollPanel.scrollHeight,
      scrollTop: scrollPanel.scrollTop,
      loadedEventCount,
    }

    loadingRef.current = true
    void onLoadMore().catch(() => {
      restoreRef.current = null
      loadingRef.current = false
    })
  }, [hasMore, isFetching, loadedEventCount, onLoadMore])

  useEffect(() => {
    const marker = markerRef.current
    const scrollPanel = marker?.parentElement
    if (!marker || !scrollPanel) return

    lastScrollTopRef.current = scrollPanel.scrollTop
    const armAndLoadIfNearTop = () => {
      armedRef.current = true
      if (scrollPanel.scrollTop <= HISTORY_TOP_LOAD_THRESHOLD) {
        loadMore()
      }
    }
    const handleScroll = () => {
      const previous = lastScrollTopRef.current ?? scrollPanel.scrollTop
      const next = scrollPanel.scrollTop
      if (shouldArmHistoryPagination(previous, next)) {
        armAndLoadIfNearTop()
      }
      lastScrollTopRef.current = next
    }
    const handleWheel = (event: WheelEvent) => {
      if (event.deltaY < 0) {
        armAndLoadIfNearTop()
      }
    }

    scrollPanel.addEventListener("scroll", handleScroll, { passive: true })
    scrollPanel.addEventListener("wheel", handleWheel, { passive: true })
    return () => {
      scrollPanel.removeEventListener("scroll", handleScroll)
      scrollPanel.removeEventListener("wheel", handleWheel)
    }
  }, [loadMore])

  useEffect(() => {
    const marker = markerRef.current
    const scrollPanel = marker?.parentElement
    if (!marker || !scrollPanel || !hasMore) return

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry?.isIntersecting) loadMore()
      },
      {
        root: scrollPanel,
        rootMargin: `${HISTORY_TOP_LOAD_THRESHOLD}px 0px 0px 0px`,
        threshold: 0,
      }
    )

    observer.observe(marker)
    return () => observer.disconnect()
  }, [hasMore, loadMore])

  useLayoutEffect(() => {
    const restore = restoreRef.current
    if (!restore || isFetching) return

    if (loadedEventCount <= restore.loadedEventCount) {
      restoreRef.current = null
      loadingRef.current = false
      return
    }

    const delta = restore.scrollPanel.scrollHeight - restore.scrollHeight
    restore.scrollPanel.scrollTop = restore.scrollTop + delta
    restoreRef.current = null
    loadingRef.current = false
  }, [isFetching, loadedEventCount])

  return (
    <>
      <div ref={markerRef} className="h-px" aria-hidden="true" />
      {isFetching ? (
        <div
          className="bg-surface/95 pointer-events-none absolute top-3 left-1/2 z-10 flex -translate-x-1/2 items-center justify-center rounded-full border border-border p-2 shadow-sm"
          role="status"
          aria-label="Loading older messages"
        >
          <Spinner size="sm" />
        </div>
      ) : null}
    </>
  )
}
