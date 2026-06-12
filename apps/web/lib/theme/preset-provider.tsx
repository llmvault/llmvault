"use client"

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react"
import {
  DEFAULT_PRESET,
  PRESET_STORAGE_KEY,
  isThemePreset,
} from "./presets"

interface PresetContextValue {
  preset: string
  setPreset: (id: string) => void
}

const PresetContext = createContext<PresetContextValue | null>(null)

export function PresetProvider({ children }: { children: React.ReactNode }) {
  const [preset, setPresetState] = useState(DEFAULT_PRESET)

  // The no-flash script in the root layout has already set the attribute
  // before paint; here we sync React state to it so the UI reflects the
  // active preset.
  useEffect(() => {
    const fromDom = document.documentElement.dataset.themePreset
    const stored =
      fromDom ??
      (typeof window !== "undefined"
        ? window.localStorage.getItem(PRESET_STORAGE_KEY)
        : null)
    if (isThemePreset(stored)) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setPresetState(stored as string)
    }
  }, [])

  const setPreset = useCallback((id: string) => {
    if (!isThemePreset(id)) return
    setPresetState(id)
    try {
      window.localStorage.setItem(PRESET_STORAGE_KEY, id)
    } catch {
      // localStorage can be unavailable (private mode); attribute still applies.
    }
    if (id === DEFAULT_PRESET) {
      delete document.documentElement.dataset.themePreset
    } else {
      document.documentElement.dataset.themePreset = id
    }
  }, [])

  return (
    <PresetContext.Provider value={{ preset, setPreset }}>
      {children}
    </PresetContext.Provider>
  )
}

export function usePreset() {
  const context = useContext(PresetContext)
  if (!context) {
    throw new Error("usePreset must be used within a PresetProvider")
  }
  return context
}
