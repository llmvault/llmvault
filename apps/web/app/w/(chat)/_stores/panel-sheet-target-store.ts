"use client"

import { create } from "zustand"

// Which sheet the shared right panel should open, when a sheet is launched
// from a non-session surface (e.g. the /w/sheets dashboard). The session
// SheetsView reads this to render a specific sheet in the existing right
// panel instead of falling back to the session's team.
interface PanelSheetTarget {
  teamId: string
  sheetId: string
}

interface PanelSheetTargetState {
  target: PanelSheetTarget | null
  openSheet: (teamId: string, sheetId: string) => void
  clear: () => void
}

export const usePanelSheetTargetStore = create<PanelSheetTargetState>((set) => ({
  target: null,
  openSheet: (teamId, sheetId) => set({ target: { teamId, sheetId } }),
  clear: () => set({ target: null }),
}))
