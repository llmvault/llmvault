"use client"

import { useCallback, useEffect, useState } from "react"
import { readMicrophonePermission } from "@/app/w/(chat)/_lib/microphone-permissions"

export function useMicrophonePermission() {
  const [micPermissionGranted, setMicPermissionGranted] = useState(false)

  useEffect(() => {
    let cancelled = false
    let permissionStatus: PermissionStatus | null = null

    void readMicrophonePermission().then((status) => {
      if (cancelled || !status) return
      permissionStatus = status
      setMicPermissionGranted(status.state === "granted")
      status.onchange = () => {
        setMicPermissionGranted(status.state === "granted")
      }
    })

    return () => {
      cancelled = true
      if (permissionStatus) {
        permissionStatus.onchange = null
      }
    }
  }, [])

  const hasGrantedMicrophonePermission = useCallback(async () => {
    if (micPermissionGranted) return true

    const status = await readMicrophonePermission()
    const granted = status?.state === "granted"
    if (granted) {
      setMicPermissionGranted(true)
    }
    return granted
  }, [micPermissionGranted])

  return { hasGrantedMicrophonePermission, setMicPermissionGranted }
}
