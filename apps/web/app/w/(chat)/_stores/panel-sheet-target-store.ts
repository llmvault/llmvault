"use client"

import { create } from "zustand"

// Which sheet the shared right panel should open, when a sheet is launched
// from a non-session surface (e.g. the /w/sheets dashboard). The session
// SheetsView reads this to render a specific sheet in the existing right
// panel instead of falling back to the session's own channel.
interface PanelSheetTarget {
  channelId: string
  sheetId: string
}

interface PanelSheetTargetState {
  target: PanelSheetTarget | null
  openSheet: (channelId: string, sheetId: string) => void
  clear: () => void
}

export const usePanelSheetTargetStore = create<PanelSheetTargetState>((set) => ({
  target: null,
  openSheet: (channelId, sheetId) => set({ target: { channelId, sheetId } }),
  clear: () => set({ target: null }),
}))
