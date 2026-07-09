"use client"

import { create } from "zustand"

// Which Canvas artifact DesignView should select when the right panel opens for
// a session — set by the session-notice dispatch on artifact.synced. DesignView
// consumes the target once, then user selection takes over.
interface PanelArtifactTarget {
  sessionId: string
  artifactId: string
}

interface PanelArtifactTargetState {
  target: PanelArtifactTarget | null
  openArtifact: (sessionId: string, artifactId: string) => void
  clear: () => void
}

export const usePanelArtifactTargetStore = create<PanelArtifactTargetState>(
  (set) => ({
    target: null,
    openArtifact: (sessionId, artifactId) =>
      set({ target: { sessionId, artifactId } }),
    clear: () => set({ target: null }),
  })
)
