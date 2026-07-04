"use client"

import { create } from "zustand"

// Which app the shared right panel should launch, when opened from a
// non-session surface (e.g. the /w/apps dashboard). AppsView reads this to
// render a specific app in the existing right panel.
interface PanelAppTarget {
  appId: string
  appName: string
}

interface PanelAppTargetState {
  target: PanelAppTarget | null
  openApp: (appId: string, appName: string) => void
  clear: () => void
}

export const usePanelAppTargetStore = create<PanelAppTargetState>((set) => ({
  target: null,
  openApp: (appId, appName) => set({ target: { appId, appName } }),
  clear: () => set({ target: null }),
}))
