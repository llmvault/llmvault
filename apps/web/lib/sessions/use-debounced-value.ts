import { useEffect, useState } from "react"

/**
 * Returns a debounced copy of `value` that only updates after `delayMs` have
 * elapsed without a change.
 *
 * Debouncing the value that drives the query collapses a burst of keystrokes
 * into a single fetch while keeping the input itself fully responsive (the
 * input stays bound to the raw state; only the query reads the debounced value).
 */
export function useDebouncedValue<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value)

  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delayMs)
    return () => clearTimeout(timer)
  }, [value, delayMs])

  return debounced
}
